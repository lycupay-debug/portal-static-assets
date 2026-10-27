package main

import (
	"context"
	"database/sql"
	"encoding/json"
)

const contentSelect = `content_id::text AS "id",author_id::text AS "authorId",title,slug,content_type AS "contentType",tags,parent_category AS "parentCategory",child_category AS "childCategory",summary,body,visibility,status,created_at AS "createdAt",updated_at AS "updatedAt",published_at AS "publishedAt",deleted_at AS "deletedAt"`

func queryJSON(ctx context.Context, db interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, query string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item map[string]any
		if err = json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
func queryJSONRows(ctx context.Context, db *sql.DB, query string, args ...any) ([]map[string]any, error) {
	return queryJSON(ctx, db, query, args...)
}
func oneJSON(ctx context.Context, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, query string, args ...any) (map[string]any, error) {
	var raw []byte
	if err := db.QueryRowContext(ctx, query, args...).Scan(&raw); err != nil {
		return nil, err
	}
	var item map[string]any
	err := json.Unmarshal(raw, &item)
	return item, err
}
func contentJSONSelect() string { return "to_jsonb(c)" }
