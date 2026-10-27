package main

import (
	"strings"
)

// resourceSearchTextSQL builds the shared text corpus used by resource matching.
func resourceSearchTextSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return "lower(coalesce(" + prefix + "title, '') || ' ' || coalesce(" + prefix + "description, '') || ' ' || coalesce(" + prefix + "original_filename, '') || ' ' || coalesce(" + prefix + "resource_type, '') || ' ' || coalesce(" + prefix + "grade, '') || ' ' || coalesce(" + prefix + "resource_url, ''))"
}

// resourceDirectSearchMatchSQL matches literal substrings, with or without whitespace.
func resourceDirectSearchMatchSQL(alias, placeholder string) string {
	text := resourceSearchTextSQL(alias)
	query := "lower(" + placeholder + ")"
	compactText := "regexp_replace(" + text + ", '[[:space:]]+', '', 'g')"
	compactQuery := "regexp_replace(" + query + ", '[[:space:]]+', '', 'g')"
	return "(" + text + " LIKE '%' || " + query + " || '%' OR " + compactText + " LIKE '%' || " + compactQuery + " || '%')"
}

// resourceFuzzySearchMatchSQL applies typo tolerance to individual fields, not a concatenated
// corpus. This prevents repeated generic words in titles/metadata from making unrelated rows match.
func resourceFuzzySearchMatchSQL(alias, placeholder string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	query := "lower(" + placeholder + ")"
	fields := []string{"title", "description", "original_filename", "resource_type", "grade", "resource_url"}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, query+" <% lower(coalesce("+prefix+field+", ''))")
	}
	// If the user supplied a filename extension, fuzzy fallback must be a close
	// filename match; otherwise one-character differences in Unit/Part labels can
	// expand into dozens of unrelated audio files.
	filename := "lower(coalesce(" + prefix + "original_filename, ''))"
	filenameFuzzy := "word_similarity(" + query + ", " + filename + ") >= 0.85"
	return "((" + query + " ~ '\\.[a-z0-9]{2,5}$' AND " + filenameFuzzy + ") OR (" + query + " !~ '\\.[a-z0-9]{2,5}$' AND (" + strings.Join(parts, " OR ") + ")))"
}

// resourceSearchSimilarityOrderSQL ranks fuzzy fallbacks by their best individual-field match.
func resourceSearchSimilarityOrderSQL(alias, placeholder string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	query := "lower(" + placeholder + ")"
	fields := []string{"title", "description", "original_filename", "resource_type", "grade", "resource_url"}
	parts := make([]string, 0, len(fields))
	for _, field := range fields {
		parts = append(parts, "word_similarity("+query+", lower(coalesce("+prefix+field+", '')))")
	}
	return "GREATEST(" + strings.Join(parts, ", ") + ") DESC"
}

// resourceSearchOrderSQL ranks literal filename matches before broader text and fuzzy matches.
// The placeholder is supplied by the caller ($1 for public search, or the active query argument).
func resourceSearchOrderSQL(placeholder string) string {
	query := "lower(" + placeholder + ")"
	compactQuery := "regexp_replace(" + query + ", '[[:space:]]+', '', 'g')"
	filename := "lower(coalesce(original_filename, ''))"
	compactFilename := "regexp_replace(" + filename + ", '[[:space:]]+', '', 'g')"
	title := "lower(coalesce(title, ''))"
	return "CASE " +
		"WHEN " + filename + " = " + query + " THEN 0 " +
		"WHEN " + compactFilename + " = " + compactQuery + " THEN 1 " +
		"WHEN " + title + " = " + query + " THEN 2 " +
		"WHEN " + filename + " LIKE '%' || " + query + " || '%' THEN 3 " +
		"WHEN " + compactFilename + " LIKE '%' || " + compactQuery + " || '%' THEN 4 " +
		"WHEN " + title + " LIKE '%' || " + query + " || '%' THEN 5 " +
		"ELSE 6 END"
}
