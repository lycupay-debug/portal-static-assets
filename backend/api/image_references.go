// image_references.go centralizes managed-image reference extraction and transaction locks.
// Content create/update/restore and permanent deletion share these locks so a cleanup
// cannot race a new reference. PostgreSQL is the source of truth for registrations.

package main

import (
	"context"
	"database/sql"
	"errors"
	"sort"
)

var errManagedImageNotRegistered = errors.New("content references an unavailable managed image")

// managedImageNames returns unique managed upload filenames in deterministic order.
func managedImageNames(body string) []string {
	matches := uploadedImageURLPattern.FindAllStringSubmatch(body, -1)
	seen := make(map[string]struct{}, len(matches))
	names := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) != 2 || !uploadedImageName.MatchString(match[1]) {
			continue
		}
		if _, exists := seen[match[1]]; exists {
			continue
		}
		seen[match[1]] = struct{}{}
		names = append(names, match[1])
	}
	sort.Strings(names)
	return names
}

// lockManagedImageNames serializes transactions that may add or remove references
// to the same upload. Sorting prevents lock-order cycles when bodies contain many images.
func lockManagedImageNames(ctx context.Context, tx *sql.Tx, names []string) error {
	ordered := append([]string(nil), names...)
	sort.Strings(ordered)
	previous := ""
	for _, name := range ordered {
		if name == previous {
			continue
		}
		previous = name
		var acquired int
		if err := tx.QueryRowContext(ctx,
			"SELECT 1 FROM (SELECT pg_advisory_xact_lock(hashtextextended($1, 0))) AS image_lock",
			"portal.uploaded_image:"+name,
		).Scan(&acquired); err != nil {
			return err
		}
	}
	return nil
}

// lockContentImageReferences locks both old and new references, then verifies
// every image in the committed replacement body still has an upload registration.
func lockContentImageReferences(ctx context.Context, tx *sql.Tx, oldBody, newBody string) error {
	oldNames := managedImageNames(oldBody)
	newNames := managedImageNames(newBody)
	allNames := append(append([]string(nil), oldNames...), newNames...)
	if err := lockManagedImageNames(ctx, tx, allNames); err != nil {
		return err
	}
	for _, name := range newNames {
		var exists bool
		if err := tx.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM portal.uploaded_images WHERE filename=$1)", name,
		).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errManagedImageNotRegistered
		}
	}
	return nil
}
