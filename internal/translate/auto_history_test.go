package translate

import (
	"context"
	"testing"

	"cnb.cool/dtapp/kai/internal/model"
)

// Issue #57: a text translated while the user is still typing must not fill the history with one
// row per pause. An automatic request (Auto) keeps what it produced in memory instead of writing it;
// the window commits it once the text has stopped changing (CommitAutoHistory). A newer request,
// automatic or not, replaces what was waiting, so a half-typed sentence is never saved.

func probeEngine() *fakeEngine {
	return &fakeEngine{name: "probe", run: func(_ context.Context, req model.TranslateRequest) (*model.TranslateResult, error) {
		return &model.TranslateResult{Result: "translated " + req.Text, From: req.From, To: req.To, Text: req.Text}, nil
	}}
}

func autoReq(id, text string, auto bool) model.TranslateRequest {
	return model.TranslateRequest{Text: text, From: model.ES, To: model.EN, RequestID: id, Auto: auto}
}

func settled(t *testing.T, em *recEmitter, id string) {
	t.Helper()
	waitUntil(t, "result of "+id, func() bool { return len(em.resultsFor(id, "probe")) == 1 })
}

func TestAutoTranslationWritesNoHistoryUntilCommitted(t *testing.T) {
	svc, em, hist := newCancelService(t, probeEngine())
	if _, err := svc.TranslateMulti(autoReq("a1", "frase automatica uno", true)); err != nil {
		t.Fatal(err)
	}
	settled(t, em, "a1")
	// waitUntil returns when the result is emitted; the history write, if any, happens before that.
	if n := historyRows(t, hist, "automatica"); n != 0 {
		t.Fatalf("%d rows before the commit, want 0", n)
	}
	if got := svc.CommitAutoHistory("a1"); got != 1 {
		t.Fatalf("CommitAutoHistory saved %d rows, want 1", got)
	}
	if n := historyRows(t, hist, "automatica"); n != 1 {
		t.Fatalf("%d rows after the commit, want 1", n)
	}
	if got := svc.CommitAutoHistory("a1"); got != 0 {
		t.Errorf("a second commit saved %d rows, want 0 (it is cleared)", got)
	}
}

func TestANewerAutoRequestReplacesWhatWasWaiting(t *testing.T) {
	svc, em, hist := newCancelService(t, probeEngine())
	_, _ = svc.TranslateMulti(autoReq("a1", "mitad de una fra", true))
	settled(t, em, "a1")
	_, _ = svc.TranslateMulti(autoReq("a2", "mitad de una frase completa", true))
	settled(t, em, "a2")
	if got := svc.CommitAutoHistory("a1"); got != 0 {
		t.Errorf("the replaced request saved %d rows, want 0", got)
	}
	if got := svc.CommitAutoHistory("a2"); got != 1 {
		t.Errorf("the newest saved %d rows, want 1", got)
	}
	if n := historyRows(t, hist, "mitad"); n != 1 {
		t.Errorf("%d rows mention the text, want exactly the complete one", n)
	}
	if n := historyRows(t, hist, "fra "); n != 0 {
		t.Errorf("the half-typed text was saved")
	}
}

func TestAManualTranslationDropsWhatWasWaitingAndSavesItself(t *testing.T) {
	svc, em, hist := newCancelService(t, probeEngine())
	_, _ = svc.TranslateMulti(autoReq("a1", "texto escrito a medias", true))
	settled(t, em, "a1")
	_, _ = svc.TranslateMulti(autoReq("m1", "texto escrito del todo", false))
	settled(t, em, "m1")
	if got := svc.CommitAutoHistory("a1"); got != 0 {
		t.Errorf("committing the dropped request saved %d rows, want 0", got)
	}
	if n := historyRows(t, hist, "del todo"); n != 1 {
		t.Errorf("the manual translation wrote %d rows, want 1", n)
	}
	if n := historyRows(t, hist, "a medias"); n != 0 {
		t.Errorf("the dropped automatic text was saved")
	}
}

func TestCommitOfAnUnknownRequestSavesNothing(t *testing.T) {
	svc, _, _ := newCancelService(t, probeEngine())
	if got := svc.CommitAutoHistory("nope"); got != 0 {
		t.Errorf("saved %d rows for an unknown request", got)
	}
	if got := svc.CommitAutoHistory(""); got != 0 {
		t.Errorf("saved %d rows for an empty id", got)
	}
}

// The pending rows belong to the NEWEST request. A result of an older one that lands late (its
// outcome was decided before a newer request opened, its stash runs after the newer one's) must not
// reset what the newer one is holding. Ordering is by the request's sequence number, not by timing.
func TestALateResultOfAnOlderRequestDoesNotReplaceTheNewerOnesRows(t *testing.T) {
	svc, _, hist := newCancelService(t, probeEngine())
	res := func(text string) *model.TranslateResult {
		return &model.TranslateResult{Engine: "probe", Text: text, Result: "t " + text, From: model.ES, To: model.EN}
	}
	svc.stashAutoHistory("A", 1, res("texto viejo uno"))
	svc.stashAutoHistory("B", 2, res("texto nuevo uno"))
	svc.stashAutoHistory("A", 1, res("texto viejo dos")) // the slower engine of A, late
	svc.stashAutoHistory("B", 2, res("texto nuevo dos")) // B's other engine
	if got := svc.CommitAutoHistory("A"); got != 0 {
		t.Errorf("the older request committed %d rows, want 0", got)
	}
	if got := svc.CommitAutoHistory("B"); got != 2 {
		t.Fatalf("the newer request committed %d rows, want both of its results", got)
	}
	if n := historyRows(t, hist, "viejo"); n != 0 {
		t.Errorf("%d rows of the older request were saved", n)
	}
	if n := historyRows(t, hist, "nuevo"); n != 2 {
		t.Errorf("%d rows of the newer request were saved, want 2", n)
	}
}

func TestALateResultOfAnOlderRequestAfterAManualOneIsIgnored(t *testing.T) {
	svc, _, _ := newCancelService(t, probeEngine())
	svc.dropAutoHistory(5) // a translation the user asked for opened as request number 5
	svc.stashAutoHistory("A", 4, &model.TranslateResult{Engine: "probe", Text: "x", Result: "y"})
	if got := svc.CommitAutoHistory("A"); got != 0 {
		t.Errorf("an older automatic request stashed after a manual one and committed %d rows, want 0", got)
	}
}

func TestRequestSequenceNumbersOnlyGrow(t *testing.T) {
	var rr requestRegistry
	a := rr.open("s", "a")
	b := rr.open("s", "b")
	c := rr.open("other", "c")
	if a.seq >= b.seq || b.seq >= c.seq {
		t.Errorf("seq = %d, %d, %d, want strictly increasing across sessions", a.seq, b.seq, c.seq)
	}
}
