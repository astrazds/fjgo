package main

import (
	"reflect"
	"testing"
)

func TestAPIBodyPreviewRedactsEncodedPageContent(t *testing.T) {
	input := apiBodyInput{JSON: map[string]any{
		"title":          "Dogfood",
		"content_base64": "cmV2ZXJzaWJsZSBjb250ZW50",
	}}
	want := map[string]any{"title": "Dogfood", "content_base64": "redacted"}
	if got := apiBodyPreview(input); !reflect.DeepEqual(got, want) {
		t.Fatalf("preview = %#v, want %#v", got, want)
	}
}
