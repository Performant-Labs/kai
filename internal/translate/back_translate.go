package translate

import (
	"log/slog"
	"strings"
	"unicode/utf8"

	"cnb.cool/dtapp/kai/internal/i18n"
	"cnb.cool/dtapp/kai/internal/model"
)

// The back-translation (issue #56).
//
// A translation can read well in its target language and still have changed the meaning. To let
// the user check it in their own language, the window can translate the displayed result back
// into the source language and show it under the result. The request is an ordinary single-engine
// translation (the text is the result, the pair is the swapped one, the engine is the one whose
// result is shown); this file is the ONE place that decides how it ends.
//
// It is display only. It is its own request (session sessionBackTranslate): a newer one replaces
// the running one, CancelTranslate cancels it, and nothing in it ever times out. It writes no
// history, changes no setting and teaches nothing to the variant store. It never returns an
// error: a back-translation that fails, is cancelled or has nothing to show must not disturb the
// main result, so the window reads only the status. Neither the text nor the answer is logged,
// only lengths and the engine's name.

// sessionBackTranslate is the request session of the back-translation. It must not collide with
// sessionTranslate, sessionContextTranslate or the screenshot sessions.
const sessionBackTranslate = "back-translate"

// BackTranslate translates req.Text (the displayed result) from req.From to req.To, with the
// engine req.EngineName. See the file comment.
func (s *Service) BackTranslate(req model.TranslateRequest) model.BackTranslateResult {
	if req.RequestID == "" {
		req.RequestID = newRequestID()
	}
	res := model.BackTranslateResult{Status: model.BackTranslateFailed, Engine: req.EngineName, RequestID: req.RequestID}
	if err := checkInputLength(req.Text); err != nil {
		return res
	}
	reg, ok := s.registry.GetTranslator(req.EngineName)
	if !ok {
		return res
	}

	ar := s.requests.open(sessionBackTranslate, req.RequestID)
	run := s.requests.start(ar, []string{req.EngineName})[0]
	defer s.requests.finish(ar, run)

	out, err := s.translateWithEngine(run.ctx, reg, req.EngineName, req, nil)
	oc := outcomeOf(run.ctx, out, err)
	switch oc.kind {
	case outcomeSuperseded:
		logEngineEnded(req.EngineName, ar, errSuperseded)
		res.Status = model.BackTranslateCancelled
	case outcomeCancelled:
		logEngineEnded(req.EngineName, ar, errUserCancelled)
		res.Status = model.BackTranslateCancelled
	case outcomeFailed:
		slog.Warn(i18n.T("log.back_translate_failed"), "engine", req.EngineName, "kind", correctionErrKind(oc.err),
			"text_len", utf8.RuneCountInString(req.Text))
	default:
		text := strings.TrimSpace(out.Result)
		if out.Identity || text == "" {
			res.Status = model.BackTranslateSkipped
			break
		}
		res.Status, res.Result = model.BackTranslateOK, text
	}
	return res
}
