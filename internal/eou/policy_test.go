package eou

import "testing"

func TestDecide(t *testing.T) {
	base := DecisionInput{
		Transcript:       "hello world",
		SilenceMS:        900,
		ElapsedMS:        3200,
		CommitThreshold:  0.6,
		HasSemanticScore: true,
		PEnd:             0.9,
	}

	t.Run("high confidence commits", func(t *testing.T) {
		decision := Decide(base)
		if !decision.ShouldCommit || decision.Reason != ReasonHighConfidenceEnd {
			t.Fatalf("unexpected decision: %+v", decision)
		}
	})

	t.Run("low confidence continues", func(t *testing.T) {
		in := base
		in.PEnd = 0.3
		decision := Decide(in)
		if decision.ShouldCommit || decision.Reason != ReasonLowConfidenceCont {
			t.Fatalf("unexpected decision: %+v", decision)
		}
	})

	t.Run("hard silence commits", func(t *testing.T) {
		in := base
		in.PEnd = 0.1
		in.SilenceMS = 3000
		decision := Decide(in)
		if !decision.ShouldCommit || decision.Reason != ReasonHardSilenceCommit {
			t.Fatalf("unexpected decision: %+v", decision)
		}
	})

	t.Run("max duration commits", func(t *testing.T) {
		in := base
		in.PEnd = 0.1
		in.ElapsedMS = 25000
		decision := Decide(in)
		if !decision.ShouldCommit || decision.Reason != ReasonMaxDurationCommit {
			t.Fatalf("unexpected decision: %+v", decision)
		}
	})

	t.Run("empty transcript continues", func(t *testing.T) {
		in := base
		in.Transcript = "   "
		decision := Decide(in)
		if decision.ShouldCommit || decision.Reason != ReasonEmptyTranscriptCont {
			t.Fatalf("unexpected decision: %+v", decision)
		}
	})

	t.Run("fallback to vad commits at candidate silence", func(t *testing.T) {
		in := base
		in.FallbackToVAD = true
		in.HasSemanticScore = false
		in.SilenceMS = 1000
		decision := Decide(in)
		if !decision.ShouldCommit || decision.Reason != ReasonFallbackVADCommit {
			t.Fatalf("unexpected decision: %+v", decision)
		}
	})
}
