package main

import (
	"strings"
	"testing"
)

func TestResourceSearchTextSQLUsesExpectedFieldsAndAlias(t *testing.T) {
	plain := resourceSearchTextSQL("")
	aliased := resourceSearchTextSQL("matched")
	for _, field := range []string{"title", "description", "original_filename", "resource_type", "grade", "resource_url"} {
		if !strings.Contains(plain, "coalesce("+field+", '')") {
			t.Fatalf("plain corpus missing %q: %s", field, plain)
		}
		if !strings.Contains(aliased, "coalesce(matched."+field+", '')") {
			t.Fatalf("aliased corpus missing %q: %s", field, aliased)
		}
	}
}

func TestResourceSearchMatchSQLPreservesLiteralAndWhitespaceFallbacks(t *testing.T) {
	query := resourceDirectSearchMatchSQL("matched", "$2")
	for _, fragment := range []string{"matched.title", "regexp_replace", "lower($2)"} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("direct matching missing %q: %s", fragment, query)
		}
	}
}

func TestResourceFuzzySearchSQLRestrictsFilenameExtensionFallback(t *testing.T) {
	query := resourceFuzzySearchMatchSQL("", "$1")
	for _, fragment := range []string{"original_filename", "word_similarity", "!~", "~"} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("fuzzy matching missing %q: %s", fragment, query)
		}
	}
}

func TestResourceSearchOrderSQLPrioritizesExactFilename(t *testing.T) {
	query := resourceSearchOrderSQL("$1")
	for _, fragment := range []string{"original_filename", "THEN 0", "THEN 2", "ELSE 6 END"} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("search order missing %q: %s", fragment, query)
		}
	}
}
