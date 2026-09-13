package engine

import "testing"

// Stage 69: books.

func bookConfig() Config {
	cfg := coinConfig()
	cfg.Books = true
	cfg.SkillBirthplace = 0.5
	return cfg
}

// A writer with something to say: one skill and a hand to write with.
func aScribe(t *testing.T, w *World, x, y float64) *Agent {
	t.Helper()
	id := w.addAgent(Agent{Maturity: 1, X: x, Y: y, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	a := mustAgent(t, w, id)
	a.hintSlots = 4
	w.learnSkill(a, SkillScribe, 1)
	w.learnSkill(a, SkillForage, 0.8)
	return a
}

func TestBooksAreOffByDefault(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Books {
		t.Fatal("books should be off by default")
	}
	w := NewWorld(cfg)
	a := w.addAgent(Agent{Maturity: 1, X: 100, Y: 100, Vitality: 90})
	if _, strength := w.worthWriting(mustAgent(t, w, a)); strength != 0 {
		t.Fatalf("with books off there is nothing to write, got %v", strength)
	}
}

// What a book says is worse than what its writer knows, and it is capped by
// what the writer's own hand can manage.
func TestABookIsWorseThanItsWriter(t *testing.T) {
	cfg := bookConfig()
	w := NewWorld(cfg)
	a := aScribe(t, w, 100, 100)

	kind, strength := w.worthWriting(a)
	if kind != SkillForage {
		t.Fatalf("it should write about the thing it knows, got %v", kind)
	}
	held := a.nominalSkill(SkillForage)
	if !(strength > 0 && strength < held) {
		t.Fatalf("a book should say less than its writer knows: wrote %v, knows %v", strength, held)
	}

	// And it never writes about writing: a line of scribes that only ever
	// wrote about scribing would say nothing about the world.
	w.learnSkill(a, SkillScribe, 1)
	if kind, _ := w.worthWriting(a); kind == SkillScribe {
		t.Fatal("a book about writing books is not what this is for")
	}
}

// A book is a thing in the hands: it takes one, weighs nothing, and nobody
// eats it.
func TestABookTakesAHandAndWeighsNothing(t *testing.T) {
	cfg := bookConfig()
	w := NewWorld(cfg)
	a := aScribe(t, w, 100, 100)
	if !w.write(a) {
		t.Fatal("it had something to say and a free hand")
	}
	if a.CarriedCount() != 1 {
		t.Fatalf("the book is not in its hands: %d held", a.CarriedCount())
	}
	if got := a.burden(&cfg); got != 1 {
		t.Fatalf("a book weighs nothing: burden %v", got)
	}
	if w.canEat(a, &a.carried[0]) {
		t.Fatal("something ate a book")
	}
	if a.canCarryMore(&cfg) {
		t.Fatal("the hand should be full: that is the price stage 67 says to watch")
	}
}

// The asymmetry the stage exists for: reading does not use the book up, and a
// body that has read one has nothing left to gain from it.
func TestAReadBookIsWorthNothingToItsReaderAndSomethingToEverybodyElse(t *testing.T) {
	cfg := bookConfig()
	w := NewWorld(cfg)
	writer := aScribe(t, w, 100, 100)
	if !w.write(writer) {
		t.Fatal("nothing written")
	}
	book := writer.carried[0]

	// Somebody who knows nothing about foraging, with room to learn.
	id := w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	reader := mustAgent(t, w, id)
	reader.hintSlots = 4
	reader.carried = append(reader.carried, book)

	before := w.bookValue(reader, &reader.carried[0])
	if before <= 0 {
		t.Fatalf("a book about something it does not know should be worth reading, got %v", before)
	}
	if !w.read(reader, 0) {
		t.Fatal("it learned nothing from a book about something it did not know")
	}
	if reader.nominalSkill(SkillForage) <= 0 {
		t.Fatal("reading taught it nothing")
	}
	// Still in its hands...
	if reader.heldBook() < 0 {
		t.Fatal("the book was used up; that is the control, not the rule")
	}
	// ... and worth nothing to it now.
	if after := w.bookValue(reader, &reader.carried[reader.heldBook()]); after != 0 {
		t.Fatalf("a book it has read should be worth nothing to it, got %v", after)
	}
	// But still worth something to somebody who has not read it.
	id2 := w.addAgent(Agent{Maturity: 1, X: 110, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	other := mustAgent(t, w, id2)
	other.hintSlots = 4
	if w.bookValue(other, &reader.carried[reader.heldBook()]) <= 0 {
		t.Fatal("the same book should still be worth something to somebody who has not read it")
	}
}

// And that is what makes a seller willing - but only half of what a sale
// needs, and the other half is stage 67.
//
// Parting with a read book costs its owner nothing, which is the thing stage
// 68 found a market never has. What it still needs is for the coin to be worth
// something to that owner, and a coin is worth the meal it will buy when the
// body runs short. In the world as it stands that figure is exactly zero for
// anybody who is not hungry - the flat gradient stage 67 was written about -
// so the trade is nothing for nothing and does not happen. Give the same body
// one step of lookahead and it does.
func TestSellingAReadBookNeedsTheBuyersCoinToBeWorthSomething(t *testing.T) {
	build := func(lookahead float64) (*World, *Agent, *Agent) {
		cfg := bookConfig()
		cfg.CoinValue, cfg.LookaheadHorizons = 0.5, lookahead
		w := NewWorld(cfg)
		seller := aScribe(t, w, 100, 100)
		if !w.write(seller) {
			t.Fatal("nothing written")
		}
		w.read(seller, seller.heldBook())
		buyer := mustAgent(t, w, holdingCoin(t, w, 105, 100))
		buyer.hintSlots = 4
		return w, seller, buyer
	}

	w, seller, buyer := build(0)
	if w.willSell(seller, buyer, &seller.carried[seller.heldBook()]) {
		t.Fatal("without lookahead a satiated body puts no value on a coin, so there is nothing to trade for")
	}

	w, seller, buyer = build(1)
	if !w.willSell(seller, buyer, &seller.carried[seller.heldBook()]) {
		t.Fatal("a book it has read costs it nothing, and with lookahead the coin is worth something")
	}
}

// A book that was never read is not free to part with: it still has something
// to say to its owner.
func TestAnUnreadBookIsNotFree(t *testing.T) {
	cfg := bookConfig()
	cfg.CoinValue = 0.5
	w := NewWorld(cfg)

	// A body that could learn from the book it is holding.
	writer := aScribe(t, w, 100, 100)
	w.write(writer)
	book := writer.carried[0]

	id := w.addAgent(Agent{Maturity: 1, X: 200, Y: 200, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	holder := mustAgent(t, w, id)
	holder.hintSlots = 4
	holder.carried = append(holder.carried, book)

	buyer := mustAgent(t, w, holdingCoin(t, w, 205, 200))
	unreadWorth := w.bookValue(holder, &holder.carried[0])
	if unreadWorth <= 0 {
		t.Fatal("the test needs a book the holder has not read")
	}
	// Whether it sells depends on what a coin is worth to it against that,
	// which is the ordinary comparison - what must hold is that the unread
	// book costs it something and the read one costs it nothing.
	w.read(holder, 0)
	if readWorth := w.bookValue(holder, &holder.carried[holder.heldBook()]); readWorth >= unreadWorth {
		t.Fatalf("reading should empty it of value to its owner: %v then %v", unreadWorth, readWorth)
	}
	_ = buyer
}

// The control: a book that is used up behaves like a meal.
func TestAConsumedBookIsGone(t *testing.T) {
	cfg := bookConfig()
	cfg.BookSurvivesReading = false
	w := NewWorld(cfg)
	writer := aScribe(t, w, 100, 100)
	w.write(writer)
	book := writer.carried[0]

	id := w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	reader := mustAgent(t, w, id)
	reader.hintSlots = 4
	reader.carried = append(reader.carried, book)
	w.read(reader, 0)
	if reader.heldBook() >= 0 {
		t.Fatal("with the control on, reading should use the book up")
	}
}

// A book about places names the caches its writer knew, and reading it is how
// somebody else comes to know them - the subject with somewhere to land.
func TestABookOfPlacesTellsSomebodyWhereTheCachesAre(t *testing.T) {
	cfg := bookConfig()
	cfg.BookSubject = BookPlaces
	w := NewWorld(cfg)
	w.SetStore(300, 300)
	w.SetStore(500, 300)

	writer := aScribe(t, w, 100, 100)
	w.learnStore(writer, 0, 10)
	w.learnStore(writer, 1, 10)
	if !w.write(writer) {
		t.Fatal("it knew two caches and had a hand free")
	}
	if got := len(writer.carried[0].Places); got != 2 {
		t.Fatalf("the book should name both caches, names %d", got)
	}

	id := w.addAgent(Agent{Maturity: 1, X: 105, Y: 100, Vitality: 90,
		Hunger: 20, Genome: genomeOf(50, 50, 50)})
	reader := mustAgent(t, w, id)
	reader.carried = append(reader.carried, writer.carried[0])
	if w.knowsStore(reader, 0) {
		t.Fatal("it should not know the cache yet")
	}
	if !w.read(reader, 0) {
		t.Fatal("it learned nothing")
	}
	if !w.knowsStore(reader, 0) || !w.knowsStore(reader, 1) {
		t.Fatal("reading should have told it where both caches are")
	}
}

// And the world runs with it on.
func TestTheWorldRunsWithBooksInIt(t *testing.T) {
	for _, subject := range []BookSubject{BookSkills, BookPlaces} {
		cfg := DefaultConfig()
		cfg.Seed = 5
		cfg.Books, cfg.BookSubject, cfg.SkillBirthplace = true, subject, 0.5
		w := NewWorld(cfg)
		w.SetStore(300, 300)
		for i := 0; i < 4000; i++ {
			w.Step()
		}
		if len(w.agents) == 0 {
			t.Fatalf("%v: the world emptied", subject)
		}
	}
}
