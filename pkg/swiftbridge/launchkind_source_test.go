package swiftbridge

import (
	"os"
	"strings"
	"testing"
)

// Issue #17: the bridge tells a login-item launch from a user launch. A TEXT check of the Swift
// source and the Go registration (CI has no Swift toolchain), like the other *_source_test.go.

func TestLaunchKindSwiftSource(t *testing.T) {
	src := readSwift(t, "apple_launchkind.swift")
	for _, want := range []string{
		`@_cdecl("kai_launch_observe")`,
		`@_cdecl("kai_launch_kind")`,
		"currentAppleEvent",
		"0x70726474", // keyAEPropData 'prdt'
		"0x6C676974", // keyAELaunchedAsLogInItem 'lgit'
		"didFinishLaunchingNotification",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("apple_launchkind.swift must contain %q", want)
		}
	}
}

func TestLaunchKindRegisteredOnBothPlatforms(t *testing.T) {
	for _, f := range []string{"load.go", "load_other.go"} {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(b), "KaiLaunchObserve") || !strings.Contains(string(b), "KaiLaunchKind") {
			t.Errorf("%s must declare KaiLaunchObserve and KaiLaunchKind", f)
		}
	}
	b, _ := os.ReadFile("load.go")
	if !strings.Contains(string(b), `"kai_launch_observe"`) || !strings.Contains(string(b), `"kai_launch_kind"`) {
		t.Error("load.go must register kai_launch_observe and kai_launch_kind")
	}
}
