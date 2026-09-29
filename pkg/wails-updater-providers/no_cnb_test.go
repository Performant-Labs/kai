package wails_updater_providers

import (
	"context"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// issue #178: the CNB source is gone. These tests pin the GitHub-only behaviour.

// No Options field may mention CNB.
func TestOptionsHaveNoCnbFields(t *testing.T) {
	typ := reflect.TypeOf(Options{})
	for i := 0; i < typ.NumField(); i++ {
		if strings.Contains(strings.ToLower(typ.Field(i).Name), "cnb") {
			t.Errorf("Options still has CNB field %q", typ.Field(i).Name)
		}
	}
}

// A settings value left over from the CNB era must not select anything: it falls back to auto.
func TestLegacyCnbSourceNormalizesToAuto(t *testing.T) {
	t.Cleanup(func() { SetSource(SourceAuto) })
	SetSource(Source("cnb"))
	if got := GetSource(); got != SourceAuto {
		t.Errorf("GetSource() after SetSource(cnb) = %q, want %q", got, SourceAuto)
	}
}

// Auto used to pick CNB for Chinese UI. With no CNB it must use GitHub for every language, and
// NewMirrorProvider must work with only GithubRepo set.
func TestAutoSourceUsesGithubForChineseLocale(t *testing.T) {
	t.Cleanup(func() { SetSource(SourceAuto); SetLocale(LocaleEnUS) })
	SetLogger(discardLogger())
	SetLocale(LocaleZhCN)
	SetSource(SourceAuto)
	var hits int32
	srv := mustServe(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		http.NotFound(w, r)
	}))
	SetClient(redirectClient(srv))
	mp, err := NewMirrorProvider(&Options{GithubRepo: testRepo, BuildTime: time.Now()})
	if err != nil {
		t.Fatalf("NewMirrorProvider with only GithubRepo (zh-CN, auto): %v", err)
	}
	if _, err := mp.Check(context.Background(), updater.CheckRequest{Platform: "darwin", Arch: "arm64", CurrentVersion: "1.0.0"}); err != nil {
		t.Fatalf("Check: %v", err)
	}
	if atomic.LoadInt32(&hits) == 0 {
		t.Error("Check did not reach the (GitHub) server")
	}
}

// A private repo (or one with no releases yet) answers 404 to anonymous callers. That must read as
// "no update", not as an error the app logs as a failure or shows to the user.
func TestNoVisibleReleasesIsQuietNoUpdate(t *testing.T) {
	t.Cleanup(func() { SetSource(SourceAuto); SetLocale(LocaleEnUS) })
	SetLogger(discardLogger())
	SetLocale(LocaleEnUS)
	SetSource(SourceGithub)
	srv := mustServe(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	SetClient(redirectClient(srv))
	for _, pre := range []bool{false, true} {
		mp, err := NewMirrorProvider(&Options{GithubRepo: testRepo, BuildTime: time.Now(), Prerelease: pre})
		if err != nil {
			t.Fatalf("NewMirrorProvider: %v", err)
		}
		rel, err := mp.Check(context.Background(), updater.CheckRequest{Platform: "darwin", Arch: "arm64", CurrentVersion: "1.0.0"})
		if err != nil || rel != nil {
			t.Errorf("prerelease=%v: 404 from the releases API gave (rel=%v, err=%v), want (nil, nil)", pre, rel, err)
		}
	}
}
