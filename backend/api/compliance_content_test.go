package main

import "testing"

func TestCopyrightReviewAllowsPublishOnlyWhenApproved(t *testing.T) {
	for _, status := range []string{"pending", "in_review", "approved_with_conditions", "replace_or_remove", "blocked", "unknown", "restricted", ""} {
		if copyrightReviewAllowsPublish(status) {
			t.Errorf("status %q must not permit publication", status)
		}
	}
	if !copyrightReviewAllowsPublish("approved") {
		t.Fatal("approved status must permit proceeding to the remaining rights checks")
	}
}
