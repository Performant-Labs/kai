package engine

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// issue #83 (Tester, RED): per-engine input budget table (brief docs/handoffs/83-brief.md, AC 1-4).
//
// Contract T assumes (F must provide, in internal/engine):
//   type BudgetUnit string   // UnitRunes, UnitUTF8Bytes, UnitQueryEscapedBytes
//   type BudgetSource string // SourceMeasured, SourceDocumented, SourceProvisional
//   type Budget struct { Unit BudgetUnit; Limit int; Source BudgetSource; FollowUp int }
//   const BudgetMarginPercent = 80
//   func (Budget) Max() int; Measure(string) int; Fits(string) bool
//   func InputBudget(engineName string) (Budget, bool)

func isKnownUnit(u BudgetUnit) bool {
	return u == UnitRunes || u == UnitUTF8Bytes || u == UnitQueryEscapedBytes
}

func TestInputBudgetUnitStrings(t *testing.T) {
	if UnitRunes != "runes" || UnitUTF8Bytes != "utf8_bytes" || UnitQueryEscapedBytes != "query_escaped_bytes" {
		t.Errorf("unit string values wrong: %q %q %q", UnitRunes, UnitUTF8Bytes, UnitQueryEscapedBytes)
	}
	if BudgetMarginPercent != 80 {
		t.Errorf("BudgetMarginPercent = %d, want 80", BudgetMarginPercent)
	}
}

func TestInputBudgetForEveryTranslator(t *testing.T) {
	names := translatorNames()
	if len(names) == 0 {
		t.Fatal("no translator engines found")
	}
	for _, name := range names {
		b, ok := InputBudget(name)
		if !ok {
			t.Errorf("engine %q has no input budget", name)
			continue
		}
		if b.Limit <= 0 {
			t.Errorf("engine %q: Limit = %d, want > 0", name, b.Limit)
		}
		if !isKnownUnit(b.Unit) {
			t.Errorf("engine %q: unknown unit %q", name, b.Unit)
		}
		if want := b.Limit * 80 / 100; b.Max() != want {
			t.Errorf("engine %q: Max() = %d, want Limit*80/100 = %d", name, b.Max(), want)
		}
		if b.Source != SourceMeasured && b.FollowUp == 0 {
			t.Errorf("engine %q: non-measured budget needs a FollowUp issue", name)
		}
		if b.Source == SourceMeasured && b.FollowUp != 0 {
			t.Errorf("engine %q: measured budget must have FollowUp 0, got %d", name, b.FollowUp)
		}
	}
}

func TestInputBudgetNoneForOCR(t *testing.T) {
	for _, name := range []string{"tesseract", "vision", "nope"} {
		if _, ok := InputBudget(name); ok {
			t.Errorf("InputBudget(%q) ok = true, want false", name)
		}
	}
}

// Issue #111 scope change ("split it"): the measured Apple budget is #119. Until it lands the apple
// row is provisional, with #119 as its follow-up.
func TestInputBudgetAppleIsProvisional(t *testing.T) {
	b, ok := InputBudget("apple")
	if !ok {
		t.Fatal("apple has no budget")
	}
	if b.Source != SourceProvisional {
		t.Errorf("apple Source = %q, want %q", b.Source, SourceProvisional)
	}
	if b.FollowUp != 119 {
		t.Errorf("apple FollowUp = %d, want 119", b.FollowUp)
	}
	if b.Unit != UnitRunes {
		t.Errorf("apple Unit = %q, want %q", b.Unit, UnitRunes)
	}
}

func TestInputBudgetProvisionalValues(t *testing.T) {
	want := map[string]Budget{
		"google":    {UnitQueryEscapedBytes, 5000, SourceProvisional, 87},
		"deepl":     {UnitQueryEscapedBytes, 131072, SourceDocumented, 88},
		"baidu":     {UnitUTF8Bytes, 6000, SourceDocumented, 89},
		"tencent":   {UnitRunes, 2000, SourceDocumented, 90},
		"youdao":    {UnitRunes, 5000, SourceDocumented, 91},
		"openai":    {UnitRunes, 4096, SourceProvisional, 92},
		"anthropic": {UnitRunes, 4096, SourceProvisional, 93},
		"gemini":    {UnitRunes, 4096, SourceProvisional, 94},
	}
	wantMax := map[string]int{
		"google": 4000, "deepl": 104857, "baidu": 4800, "tencent": 1600,
		"youdao": 4000, "openai": 3276, "anthropic": 3276, "gemini": 3276,
	}
	for name, w := range want {
		got, ok := InputBudget(name)
		if !ok {
			t.Errorf("%s: no budget", name)
			continue
		}
		if got != w {
			t.Errorf("%s: budget = %+v, want %+v", name, got, w)
		}
		if got.Max() != wantMax[name] {
			t.Errorf("%s: Max() = %d, want %d", name, got.Max(), wantMax[name])
		}
	}
}

func TestBudgetMeasure(t *testing.T) {
	cases := []struct {
		unit BudgetUnit
		text string
		want int
	}{
		{UnitRunes, "中a", 2},
		{UnitUTF8Bytes, "中a", 4},
		{UnitQueryEscapedBytes, "中", 9},
		{UnitQueryEscapedBytes, "a b", 3},
		{UnitQueryEscapedBytes, "a&b", 5},
	}
	for _, c := range cases {
		if got := (Budget{Unit: c.unit}).Measure(c.text); got != c.want {
			t.Errorf("Measure(%q) in %s = %d, want %d", c.text, c.unit, got, c.want)
		}
	}

	for _, u := range []BudgetUnit{UnitRunes, UnitUTF8Bytes, UnitQueryEscapedBytes} {
		b := Budget{Unit: u, Limit: 100, Source: SourceProvisional, FollowUp: 1}
		if b.Max() != 80 {
			t.Fatalf("%s: Max() = %d, want 80", u, b.Max())
		}
		atMax := strings.Repeat("a", b.Max())
		if !b.Fits(atMax) {
			t.Errorf("%s: Fits at exactly Max() = false, want true", u)
		}
		if b.Fits(atMax + "a") {
			t.Errorf("%s: Fits at Max()+1 = true, want false", u)
		}
	}
}

// docRows returns the table rows of docs/engine-limits.md as trimmed cells.
func docRows(t *testing.T) [][]string {
	t.Helper()
	data, err := os.ReadFile("../../docs/engine-limits.md")
	if err != nil {
		t.Fatalf("read docs/engine-limits.md: %v", err)
	}
	var rows [][]string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") {
			continue
		}
		parts := strings.Split(strings.Trim(line, "|"), "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		rows = append(rows, parts)
	}
	return rows
}

// Columns: Engine | Unit | Latin max | CJK max | Failure past the limit | Latency at max |
// Documented limit | Budget (80%) | Verified | Date | Follow-up
const (
	colEngine     = 0
	colDocumented = 6
	colVerified   = 8
	colFollowUp   = 10
	docColumns    = 11
)

func TestEngineLimitsDocListsEveryTranslator(t *testing.T) {
	rows := docRows(t)
	for _, name := range translatorNames() {
		var matches [][]string
		for _, r := range rows {
			if len(r) > 0 && r[colEngine] == name {
				matches = append(matches, r)
			}
		}
		if len(matches) != 1 {
			t.Errorf("engine %q: %d table rows in docs/engine-limits.md, want exactly 1", name, len(matches))
			continue
		}
		r := matches[0]
		if len(r) != docColumns {
			t.Errorf("engine %q: row has %d cells, want %d", name, len(r), docColumns)
			continue
		}
		b, ok := InputBudget(name)
		if !ok {
			t.Errorf("engine %q: no budget to check the doc row against", name)
			continue
		}
		if r[colVerified] != "no" {
			t.Errorf("%s: Verified cell = %q, want %q", name, r[colVerified], "no")
		}
		if want := fmt.Sprintf("#%d", b.FollowUp); !strings.Contains(r[colFollowUp], want) {
			t.Errorf("%s: Follow-up cell %q does not contain %s", name, r[colFollowUp], want)
		}
		// Apple publishes no limit, so its documented-limit cell says so instead of linking.
		if name != "apple" && !strings.Contains(r[colDocumented], "http") {
			t.Errorf("%s: Documented-limit cell %q has no http link", name, r[colDocumented])
		}
	}
}

func TestBudgetMeasureUnknownUnitPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Measure with an unknown unit did not panic")
		}
	}()
	Budget{Unit: "nope"}.Measure("x")
}

// Issue #111 criterion 11 (Tester). The old 650-rune limit was the bridge's 20 s wait, and the
// largest size that wait ever let through was 2250 runes (Latin). With no wait, the provisional
// limit (throwaway checks, #119 measures it) must be above that.
func TestInputBudgetAppleLimitIsNotTheOldWait(t *testing.T) {
	b, ok := InputBudget("apple")
	if !ok {
		t.Fatal("apple has no budget")
	}
	if b.Limit <= 2250 {
		t.Errorf("apple Limit = %d, want > 2250 (the largest size the 20 s wait ever let through)", b.Limit)
	}
	if b.Source != SourceProvisional {
		t.Errorf("apple Source = %q, want %q", b.Source, SourceProvisional)
	}
}

var firstInt = regexp.MustCompile(`\d[\d,]*`)

func cellInt(t *testing.T, what, cell string) int {
	t.Helper()
	m := firstInt.FindString(cell)
	if m == "" {
		t.Fatalf("%s cell %q holds no number", what, cell)
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m, ",", ""))
	if err != nil {
		t.Fatalf("%s cell %q: %v", what, cell, err)
	}
	return n
}

// While the apple row is provisional (#119 measures it), its doc row must be tied to the code: the
// Latin max is Limit, the Budget is Max(), the CJK max is written as a lower bound (">= N") because it
// does not set Limit ("Reading the table" in docs/engine-limits.md), and the row no longer blames the
// removed 20 s wait. When #119 measures the row the limit becomes the smaller of the two maximums, and
// this test moves with it.
func TestEngineLimitsDocApplePinsTheCode(t *testing.T) {
	const (
		colLatin  = 2
		colCJK    = 3
		colFailed = 4
		colBudget = 7
	)
	var row []string
	for _, r := range docRows(t) {
		if len(r) > 0 && r[colEngine] == "apple" {
			row = r
		}
	}
	if len(row) != docColumns {
		t.Fatalf("apple row has %d cells, want %d", len(row), docColumns)
	}
	b, ok := InputBudget("apple")
	if !ok {
		t.Fatal("apple has no budget")
	}
	// The provisional figure comes from the Latin throwaway check; the CJK cell is a lower bound
	// from a smaller throwaway check, so only the Latin cell is tied to Limit.
	if latin := cellInt(t, "Latin max", row[colLatin]); latin != b.Limit {
		t.Errorf("doc Latin max %d, code Limit is %d", latin, b.Limit)
	}
	if !strings.HasPrefix(row[colCJK], ">=") {
		t.Errorf("doc CJK max %q: a provisional row writes it as a lower bound (\">= N\"), because it does not set Limit", row[colCJK])
	}
	if got := cellInt(t, "Budget", row[colBudget]); got != b.Max() {
		t.Errorf("doc Budget %d, code Max() = %d", got, b.Max())
	}
	if strings.Contains(row[colFailed], "20 s") || strings.Contains(row[colFailed], "returned empty") {
		t.Errorf("apple row still describes the removed 20 s wait as the failure: %q", row[colFailed])
	}
}
