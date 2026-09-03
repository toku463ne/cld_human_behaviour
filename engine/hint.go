package engine

import "math"

// Hints are the part of what an agent has learned that the designer did not
// write down (stage 12c).
//
// Everything else in the utility formula is a relation somebody worked out and
// wrote as a closed expression: being low on vitality is dangerous, a rival
// standing over your food costs you the race, resting where strangers can see
// you is not resting. Those cover the relations that were noticed. A hint is
// for the ones that were not: a situation, an action, and a weight, discovered
// by selection rather than by design.
//
// The one thing a hint may never do is decide anything. It is added to an
// option's score and nothing else - the option still has to win the same
// comparison against the same rivals, and a hint pushing hard for a move that
// would kill the agent loses to the life term the way anything else would.
// There is no branch anywhere that reads a hint and acts. If that ever stops
// being true, the design has gone wrong (see "no hardcoded thresholds" in
// CLAUDE.md).
//
// Nor are they gated on intelligence or rationality. That was tried once, with
// the strategy depth gate, and it did not work: a hard threshold on a
// continuous ability lands in the wrong place and stays there. What a hint
// costs is budget, which is the same thing everything else costs.

// HintFeature is what a hint reads off the situation. Every one of these comes
// out of Perception, so a hint can never see anything the agent cannot, and
// every one is scaled to about 0..1 so that one range of weights does for all
// of them.
type HintFeature uint8

const (
	HintHunger    HintFeature = iota // how hungry it is
	HintWorn                         // how far below full its vitality has fallen
	HintScarcity                     // how contested the neighbourhood feels
	HintCrowd                        // how many others are in sight
	HintStrength                     // how hard it reckons the target hits
	HintCloseness                    // how near the target is
	HintTrust                        // how much good it remembers of the target
	HintOtherKind                    // whether the target is a different creature

	NumHintFeatures
)

func (f HintFeature) String() string {
	switch f {
	case HintHunger:
		return "hungry"
	case HintWorn:
		return "worn"
	case HintScarcity:
		return "scarcity"
	case HintCrowd:
		return "crowd"
	case HintStrength:
		return "strength"
	case HintCloseness:
		return "closeness"
	case HintTrust:
		return "trust"
	case HintOtherKind:
		return "other kind"
	}
	return "?"
}

// Hint is one rule of thumb: in so far as this is the case, that move is worth
// this much more (or, for a negative weight, this much less).
type Hint struct {
	Feature HintFeature
	Act     ActionKind
	Weight  float64
}

// hintFeatures is the situation as the hints read it, filled in once per
// decision for the parts that are about the agent and once per candidate for
// the parts that are about who it is looking at. An option with no other agent
// in it reads zero for the last four, which is what makes a hint about a
// stranger's strength silent when there is no stranger.
type hintFeatures [NumHintFeatures]float64

// readSelf fills in the half of the situation that is the same for every
// option this tick.
func (f *hintFeatures) readSelf(p *Perception) {
	cfg := p.Cfg
	s := &p.Self
	*f = hintFeatures{}
	f[HintHunger] = clamp(s.Hunger/cfg.MaxHunger, 0, 1)
	f[HintWorn] = clamp(1-s.Vitality/s.MaxVitality, 0, 1)
	f[HintScarcity] = clamp(s.FoodScarcity/3, 0, 1)
	f[HintCrowd] = clamp(float64(len(p.Others))/6, 0, 1)
}

// readTarget fills in the half that is about who the option is aimed at, and
// clears it again for the options that are aimed at nobody.
func (f *hintFeatures) readTarget(p *Perception, o *AgentView) {
	if o == nil {
		f[HintStrength], f[HintCloseness], f[HintTrust], f[HintOtherKind] = 0, 0, 0, 0
		return
	}
	cfg := p.Cfg
	f[HintStrength] = clamp(o.EstStrength/MaxAbility, 0, 1)
	f[HintCloseness] = clamp(1-o.Dist/cfg.PerceptionRadius, 0, 1)
	f[HintTrust] = clamp(o.Affinity/cfg.AffinityTrust, 0, 1)
	f[HintOtherKind] = boolValue(o.Species != p.Self.Species)
}

// score is what this agent's hints add to an option of the given kind. It is
// the only place a hint touches a decision, and all it does is add.
func (f *hintFeatures) score(hints []Hint, kind ActionKind) float64 {
	total := 0.0
	for i := range hints {
		if h := &hints[i]; h.Act == kind {
			total += h.Weight * f[h.Feature]
		}
	}
	return total
}

// --- where hints come from --------------------------------------------------

// drawHints gives a founder a set of rules of thumb to be judged on. They are
// random: nobody starts out knowing anything, and what survives is whatever
// happened to help.
//
// How many it starts with is drawn separately from how much room it bought,
// and can be fewer. That is not decoration: an agent with an empty slot is the
// only kind that can be taught anything (exchangeHints), so a first generation
// that filled every slot it paid for would leave the copying with nowhere to
// put an idea, and the cultural half of the stage would never fire at all.
func (w *World) drawHints(slots int) []Hint {
	if slots <= 0 {
		return nil
	}
	held := w.rng.Intn(slots + 1)
	out := make([]Hint, held)
	for i := range out {
		out[i] = w.drawHint()
	}
	return out
}

func (w *World) drawHint() Hint {
	return Hint{
		Feature: HintFeature(w.rng.Intn(int(NumHintFeatures))),
		Act:     ActionKind(w.rng.Intn(int(numActionKinds))),
		Weight:  w.rng.NormFloat64() * w.cfg.HintWeightStd,
	}
}

// drawHintSlots is how much room for rules of thumb a founder is born with.
// Uniform over the range rather than centred, so that the first generation
// contains agents that carry none at all and agents that carry the maximum:
// the whole question is what that room is worth against what it costs, and a
// population that all bought the same amount cannot answer it.
func (w *World) drawHintSlots() int {
	if w.cfg.HintSlots <= 0 {
		return 0
	}
	return w.rng.Intn(w.cfg.HintSlots + 1)
}

// inheritHints is the particulate inheritance of stage 7b, applied a slot at a
// time: each slot comes whole from one parent or the other, and now and then
// one of them changes.
//
// The mutation is in two steps, and they are deliberately different sizes. A
// weight moves the way a gene does - the ordinary mutation, common and small
// relative to what it changes. What a hint is *about* - which situation, which
// move - can only change on the rare, large event the world already has for
// exactly this: a genius birth. A new idea is a different kind of event from a
// held idea being held a little more strongly, and the world already had a
// name for it.
func (w *World) inheritHints(pa, pb *Agent, slots int, genius bool) []Hint {
	if slots <= 0 {
		return nil
	}
	out := make([]Hint, 0, slots)
	for i := 0; i < slots; i++ {
		from := pa
		if w.rng.Intn(2) == 1 {
			from = pb
		}
		// A parent with less room than its child has nothing to put in the
		// later slots. The child is left with an empty one, which is what it
		// can be taught into (exchangeHints).
		if i >= len(from.hints) {
			continue
		}
		h := from.hints[i]
		if w.cfg.MutationRate > 0 && w.rng.Float64() < w.cfg.MutationRate {
			h.Weight += w.rng.NormFloat64() * w.cfg.HintWeightStd
		}
		h.Weight = clamp(h.Weight, -w.cfg.HintWeightMax, w.cfg.HintWeightMax)
		out = append(out, h)
	}
	if genius && len(out) > 0 {
		// The leap: one of the ideas this child inherited is about something
		// else entirely.
		out[w.rng.Intn(len(out))] = w.drawHint()
	}
	return out
}

// inheritHintSlots is how much room a child is born with: one parent's, and
// now and then one more. Room is what costs, so this is the number that the
// budget is charged for.
func (w *World) inheritHintSlots(pa, pb *Agent, genius bool) int {
	slots := pa.hintSlots
	if w.rng.Intn(2) == 1 {
		slots = pb.hintSlots
	}
	if genius && slots < w.cfg.HintSlots {
		slots++
	}
	return min(slots, w.cfg.HintSlots)
}

// hintCost is what the room for rules of thumb takes out of the budget the
// genes are then fitted to. It is the whole economy of the thing: an agent
// with four ideas is measurably smaller, slower or weaker than one with none,
// and whether that trade is worth making is what selection is being asked.
func (w *World) hintCost(slots int) float64 {
	return float64(slots) * w.cfg.HintSlotCost
}

// exchangeHints is the part of a trade that is not a meeting in the middle.
// A rule of thumb is not a number two agents can average - half of "go for the
// big ones when you are starving" is not a weaker version of it, it is
// nothing - so what happens instead is that an agent with room to spare takes
// on one it did not have. This is the cultural half of stage 12b: a good trick
// spreads by being copied, and it spreads only into agents that paid for
// somewhere to put it.
func (w *World) exchangeHints(a, o *Agent) int {
	if !w.cfg.HintsSpread {
		return 0
	}
	copied := 0
	for _, h := range o.hints {
		if len(a.hints) >= a.hintSlots {
			break
		}
		if a.holdsHintLike(h) {
			continue
		}
		a.hints = append(a.hints, h)
		w.hintsCopied++
		copied++
	}
	return copied
}

// holdsHintLike reports whether this agent already has an idea about the same
// situation and the same move. What it thinks about it may be the opposite;
// being taught the same rule with the other sign is not new room used up, it
// is a disagreement, and disagreements are settled by which agent lives.
func (a *Agent) holdsHintLike(h Hint) bool {
	for i := range a.hints {
		if a.hints[i].Feature == h.Feature && a.hints[i].Act == h.Act {
			return true
		}
	}
	return false
}

// --- reading it out ---------------------------------------------------------

// Hints returns the rules of thumb this agent is carrying, and how much room it
// paid for. Read only, for the viewer.
func (a *Agent) Hints() ([]Hint, int) {
	out := make([]Hint, len(a.hints))
	copy(out, a.hints)
	return out, a.hintSlots
}

// HintUse is what a population is making of its rules of thumb.
type HintUse struct {
	Slots float64 // room bought, per agent
	Held  float64 // ideas actually carried, per agent

	// Kinds is how many distinct (situation, move) pairs are alive in the
	// population at all, and Entropy how evenly they are spread over the
	// pairs that exist. Both are needed: a population can carry plenty of
	// hints and have them all be the same one.
	Kinds   float64
	Entropy float64
}

// hintsCopiedCount is how many ideas have passed from one agent to another
// over the whole run. Nothing reads it: it is there because a rule that hardly
// ever fires explains nothing, whatever its weight.
func (w *World) hintsCopiedCount() int { return w.hintsCopied }

// HintUse reports what the living population is carrying. Read only.
func (w *World) HintUse() HintUse {
	var out HintUse
	n := 0.0
	counts := make(map[int]int)
	total := 0
	for i := range w.agents {
		a := &w.agents[i]
		if !a.Alive {
			continue
		}
		n++
		out.Slots += float64(a.hintSlots)
		out.Held += float64(len(a.hints))
		for _, h := range a.hints {
			counts[int(h.Feature)*int(numActionKinds)+int(h.Act)]++
			total++
		}
	}
	if n == 0 {
		return out
	}
	out.Slots /= n
	out.Held /= n
	out.Kinds = float64(len(counts))
	if total > 0 {
		// Shannon entropy in bits over the kinds actually in use. A
		// population all repeating one rule scores 0 however many copies of
		// it there are; the ceiling is log2 of the number of possible kinds.
		for _, c := range counts {
			p := float64(c) / float64(total)
			out.Entropy -= p * log2(p)
		}
	}
	return out
}

func log2(x float64) float64 { return math.Log2(x) }
