package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoMakeResource_CreatesTransformer(t *testing.T) {
	dir := withTempRoot(t)
	t.Setenv("APP_NAME", "testapp")

	if err := doMakeResource("blog-post"); err != nil {
		t.Fatalf("doMakeResource: %v", err)
	}

	path := filepath.Join(dir, "resources", "blog_post_resource.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("resource file not created: %v", err)
	}
	res := string(data)
	for _, want := range []string{
		"package resources",
		"// BlogPostResource shapes a blog-post model for API responses",
		"type BlogPostResource struct {",
		"func NewBlogPostResource(m /* TODO: models.BlogPost */ any) *BlogPostResource",
		"func NewBlogPostResources(items /* TODO: []models.BlogPost */ []any) []*BlogPostResource",
	} {
		if !strings.Contains(res, want) {
			t.Errorf("generated resource missing %q:\n%s", want, res)
		}
	}
	if strings.Contains(res, "$") {
		t.Errorf("generated resource still contains unreplaced placeholder:\n%s", res)
	}
}

func TestDoMakeResource_DuplicateName(t *testing.T) {
	withTempRoot(t)

	if err := doMakeResource("post"); err != nil {
		t.Fatalf("first doMakeResource: %v", err)
	}
	err := doMakeResource("post")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error, got %v", err)
	}
}

func TestDoMakeAPI_WithResource(t *testing.T) {
	dir := withTempRoot(t)
	t.Setenv("APP_NAME", "testapp")

	if err := doMakeAPI("posts", true); err != nil {
		t.Fatalf("doMakeAPI: %v", err)
	}

	// the handler references the resource and imports the resources package
	handler := readJobFile(t, filepath.Join(dir, "handlers", "api_posts.go"))
	for _, want := range []string{
		"\"testapp/resources\"",
		"resources.NewPostsResources(nil)",
		"resources.NewPostsResource(nil)",
		"resources.NewPostsResources(items), p.Meta(total)",
	} {
		if !strings.Contains(handler, want) {
			t.Errorf("generated handler missing %q:\n%s", want, handler)
		}
	}
	if strings.Contains(handler, "$") {
		t.Errorf("generated handler still contains unreplaced placeholder:\n%s", handler)
	}

	// the resource file was generated alongside
	resPath := filepath.Join(dir, "resources", "posts_resource.go")
	if _, err := os.Stat(resPath); err != nil {
		t.Fatalf("resource file not created: %v", err)
	}

	// the route is still mounted and the Scalar spec wired
	routes := readJobFile(t, filepath.Join(dir, "routes-api.go"))
	if !strings.Contains(routes, `r.Mount("/posts", a.Handlers.PostsRoutes())`) {
		t.Errorf("routes-api.go missing mount line:\n%s", routes)
	}
	if !strings.Contains(routes, "a.App.Scalar.Spec = a.Handlers.PostsAPIDocument(a.App.Server.URL)") {
		t.Errorf("routes-api.go missing Scalar spec line:\n%s", routes)
	}
}

func TestDoMakeAPI_WithoutResource(t *testing.T) {
	dir := withTempRoot(t)
	t.Setenv("APP_NAME", "testapp")

	if err := doMakeAPI("posts", false); err != nil {
		t.Fatalf("doMakeAPI: %v", err)
	}

	handler := readJobFile(t, filepath.Join(dir, "handlers", "api_posts.go"))
	if strings.Contains(handler, "resources") {
		t.Errorf("handler without --with-resource should not reference resources:\n%s", handler)
	}

	resPath := filepath.Join(dir, "resources", "posts_resource.go")
	if _, err := os.Stat(resPath); !os.IsNotExist(err) {
		t.Errorf("resource file should not exist without --with-resource")
	}
}

func TestDoMakeAPI_WithResource_DuplicateResourceRefused(t *testing.T) {
	withTempRoot(t)
	t.Setenv("APP_NAME", "testapp")

	if err := doMakeResource("posts"); err != nil {
		t.Fatalf("doMakeResource: %v", err)
	}
	// the handler file does not exist yet, but the resource does
	err := doMakeAPI("posts", true)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected already-exists error for the resource, got %v", err)
	}
}
