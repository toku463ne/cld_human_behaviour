package main

// The art (TODO 10). One character to a pixel, sixteen to a side, and a
// legend at the top of main.go: '.' nothing, 'o' the outline, 'b' the body,
// 's' a shade of it, 'e' an eye or a hole.
//
// Everything is grey on purpose. A body is tinted by its sex and by a shade
// of its own, and a meal by what kind of meal it is, so a picture with colour
// in it would need drawing once per case (sixty bodies, eight kinds of thing
// lying about).
//
// The animations are two frames each, which is as much as this earns. What
// the eye needs from this screen is "which of these is walking, which is
// eating and which is fighting" - three shapes, not three performances - and
// a body on this map is sixteen pixels across. Where there is no art for a
// state the walk is used, so nothing has to be drawn twice to be usable.
//
// Faces are deliberately plain. This is not a world where anybody is looking
// at anybody's expression: what a body knows about another is its build, and
// the build is in the size the viewer draws it at, not in the picture.

func sheet() []clipArt {
	return []clipArt{
		{"human.idle", [][]string{humanIdle1, humanIdle2}},
		{"human.walk", [][]string{humanWalk1, humanWalk2}},
		{"human.eat", [][]string{humanEat1, humanEat2}},
		{"human.fight", [][]string{humanFight1, humanFight2}},
		{"enemy.idle", [][]string{enemyIdle1, enemyIdle2}},
		{"enemy.walk", [][]string{enemyWalk1, enemyWalk2}},
		{"enemy.eat", [][]string{enemyEat1, enemyEat2}},
		{"enemy.fight", [][]string{enemyFight1, enemyFight2}},
		// The things lying about, in the order the engine numbers them
		// (FoodPlant first, and the stone, the coin, the book, the ornament
		// and the hide after the edible ones). A frame each, no animation:
		// nothing lying on the ground does anything.
		{"item", [][]string{itemPlant, itemFish, itemMeat, itemStone,
			itemCoin, itemBook, itemTrinket, itemHide}},
	}
}

// A person: a head, a body, two legs. Standing, walking, bending to a meal,
// and swinging at somebody.

var humanIdle1 = []string{
	"................",
	"......oooo......",
	".....obbbbo.....",
	".....obebbo.....",
	".....obbbbo.....",
	"......oooo......",
	"....oobbbboo....",
	"...obbbbbbbbo...",
	"...obbbbbbbbo...",
	"...obsbbbbsbo...",
	"....obbbbbbo....",
	".....obbbbo.....",
	".....ob..bo.....",
	".....ob..bo.....",
	"....oo....oo....",
	"................",
}

var humanIdle2 = []string{
	"................",
	"................",
	"......oooo......",
	".....obbbbo.....",
	".....obebbo.....",
	".....obbbbo.....",
	"......oooo......",
	"...oobbbbbboo...",
	"...obbbbbbbbo...",
	"...obsbbbbsbo...",
	"....obbbbbbo....",
	".....obbbbo.....",
	".....ob..bo.....",
	".....ob..bo.....",
	"....oo....oo....",
	"................",
}

var humanWalk1 = []string{
	"................",
	"......oooo......",
	".....obbbbo.....",
	".....obebbo.....",
	".....obbbbo.....",
	"......oooo......",
	"....oobbbboo....",
	"...obbbbbbbbo...",
	"...obbbbbbbbo...",
	"...obsbbbbsbo...",
	"....obbbbbbo....",
	".....obbbbo.....",
	"....ob....bo....",
	"...ob......bo...",
	"..oo........oo..",
	"................",
}

var humanWalk2 = []string{
	"................",
	"......oooo......",
	".....obbbbo.....",
	".....obebbo.....",
	".....obbbbo.....",
	"......oooo......",
	"..oobbbbbbbbo...",
	"..obbbbbbbbbo...",
	"...obbbbbbbbo...",
	"...obsbbbbsbo...",
	"....obbbbbbo....",
	".....obbbbo.....",
	".....ob.bo......",
	".....ob.bo......",
	"....oo..oo......",
	"................",
}

var humanEat1 = []string{
	"................",
	"................",
	"......oooo......",
	".....obbbbo.....",
	".....obebbo.....",
	".....obbbbo.....",
	"......oooo......",
	"....oobbbboo....",
	"...obbbbbbbbo...",
	"...obsbbbbsbo...",
	"....obbbbbbo....",
	".....obbbbo.....",
	".....ob..bo.....",
	".....ob..bo.....",
	"....oo....oo....",
	"................",
}

var humanEat2 = []string{
	"................",
	"................",
	"................",
	"......oooo......",
	".....obbbbo.....",
	".....obebbo.....",
	"......oooo......",
	"....oobbbboo....",
	"...obbbbbbbbo...",
	"...obsbbbbsbo...",
	"....obbbbbbo....",
	".....obbbbo.....",
	".....ob..bo.....",
	".....ob..bo.....",
	"....oo....oo....",
	"................",
}

var humanFight1 = []string{
	"..........oo....",
	"......oooobbo...",
	".....obbbbbbo...",
	".....obebbo.....",
	".....obbbbo.....",
	"......oooo......",
	"....oobbbboo....",
	"...obbbbbbbbo...",
	"...obbbbbbbbo...",
	"...obsbbbbsbo...",
	"....obbbbbbo....",
	".....obbbbo.....",
	".....ob..bo.....",
	".....ob..bo.....",
	"....oo....oo....",
	"................",
}

var humanFight2 = []string{
	"................",
	"......oooo......",
	".....obbbbo.....",
	".....obebbo.....",
	".....obbbbo.....",
	"......oooo......",
	"....oobbbboooo..",
	"...obbbbbbbbbbo.",
	"...obbbbbbbboo..",
	"...obsbbbbsbo...",
	"....obbbbbbo....",
	".....obbbbo.....",
	".....ob..bo.....",
	".....ob..bo.....",
	"....oo....oo....",
	"................",
}

// A beast: four legs, two eyes and a head at one end. It faces the same way a
// person does (the viewer flips it when the thing is going the other way), so
// the head is on the left.

var enemyIdle1 = []string{
	"................",
	"................",
	"...oo......oo...",
	"..obbo....obbo..",
	"..obbbooooobbo..",
	"..obbbbbbbbbbo..",
	".oobebbbbbbebo..",
	".obbbbbbbbbbbbo.",
	".obbbbbbbbbbbbo.",
	"..obbbbbbbbbbo..",
	"...oobooooboo...",
	"....ob....bo....",
	"....ob....bo....",
	"....oo....oo....",
	"................",
	"................",
}

var enemyIdle2 = []string{
	"................",
	"...oo......oo...",
	"..obbo....obbo..",
	"..obbbooooobbo..",
	"..obbbbbbbbbbo..",
	".oobebbbbbbebo..",
	".obbbbbbbbbbbbo.",
	".obbbbbbbbbbbbo.",
	"..obbbbbbbbbbo..",
	"...oobooooboo...",
	"....ob....bo....",
	"....ob....bo....",
	"....oo....oo....",
	"................",
	"................",
	"................",
}

var enemyWalk1 = []string{
	"................",
	"................",
	"...oo......oo...",
	"..obbo....obbo..",
	"..obbbooooobbo..",
	"..obbbbbbbbbbo..",
	".oobebbbbbbebo..",
	".obbbbbbbbbbbbo.",
	".obbbbbbbbbbbbo.",
	"..obbbbbbbbbbo..",
	"...oobooooboo...",
	"...ob......bo...",
	"..ob........bo..",
	"..oo........oo..",
	"................",
	"................",
}

var enemyWalk2 = []string{
	"................",
	"................",
	"...oo......oo...",
	"..obbo....obbo..",
	"..obbbooooobbo..",
	"..obbbbbbbbbbo..",
	".oobebbbbbbebo..",
	".obbbbbbbbbbbbo.",
	".obbbbbbbbbbbbo.",
	"..obbbbbbbbbbo..",
	"...oobooooboo...",
	"....ob....bo....",
	".....ob..bo.....",
	".....oo..oo.....",
	"................",
	"................",
}

var enemyEat1 = []string{
	"................",
	"................",
	"................",
	"...oo......oo...",
	"..obbo....obbo..",
	"..obbbooooobbo..",
	"..obbbbbbbbbbo..",
	".oobebbbbbbebo..",
	".obbbbbbbbbbbbo.",
	"..obbbbbbbbbbo..",
	"...oobooooboo...",
	"....ob....bo....",
	"....ob....bo....",
	"....oo....oo....",
	"................",
	"................",
}

var enemyEat2 = []string{
	"................",
	"................",
	"................",
	"................",
	"...oo......oo...",
	"..obbo....obbo..",
	"..obbbooooobbo..",
	".oobebbbbbbebo..",
	".obbbbbbbbbbbbo.",
	"..obbbbbbbbbbo..",
	"...oobooooboo...",
	"....ob....bo....",
	"....ob....bo....",
	"....oo....oo....",
	"................",
	"................",
}

var enemyFight1 = []string{
	"................",
	"................",
	"...oo......oo...",
	"..obbo....obbo..",
	"..obbbooooobbo..",
	".oobbbbbbbbbbo..",
	"oebbebbbbbbebo..",
	"obbbbbbbbbbbbbo.",
	"oebbbbbbbbbbbbo.",
	"..obbbbbbbbbbo..",
	"...oobooooboo...",
	"....ob....bo....",
	"....ob....bo....",
	"....oo....oo....",
	"................",
	"................",
}

var enemyFight2 = []string{
	"................",
	"...oo......oo...",
	"..obbo....obbo..",
	"..obbbooooobbo..",
	"oeobbbbbbbbbbo..",
	"obbbbbbbbbbbbo..",
	"oeobebbbbbbebo..",
	".obbbbbbbbbbbbo.",
	".obbbbbbbbbbbbo.",
	"..obbbbbbbbbbo..",
	"...oobooooboo...",
	"...ob......bo...",
	"..ob........bo..",
	"..oo........oo..",
	"................",
	"................",
}

// The things lying about. They are small on the screen - a meal is three
// pixels across at the far zoom - so these are read as shapes and not as
// pictures: a sprig, a fish, a joint, a lump, a disc, a slab, a gem, a skin.

var itemPlant = []string{
	"................",
	"................",
	"................",
	"......o.o.......",
	".....obobo......",
	"....obbbbbo.....",
	"....obbbbbo.....",
	".....obbbo......",
	"......oso.......",
	"......oso.......",
	".....ooooo......",
	"................",
	"................",
	"................",
	"................",
	"................",
}

var itemFish = []string{
	"................",
	"................",
	"................",
	"................",
	".....oooo...oo..",
	"...oobbbbo.obbo.",
	"..obebbbbboobbo.",
	"..obbbbbbbbbbbo.",
	"..obbbbbbbboobo.",
	"...oobbbbo.obbo.",
	".....oooo...oo..",
	"................",
	"................",
	"................",
	"................",
	"................",
}

var itemMeat = []string{
	"................",
	"................",
	"................",
	".......oo.......",
	"......obbo......",
	".....obbbbo.....",
	"....obbbbbbo....",
	"....obsbbsbo....",
	"....obbbbbbo....",
	".....obbbbo.....",
	"......oooo......",
	"......o..o......",
	"......oooo......",
	"................",
	"................",
	"................",
}

var itemStone = []string{
	"................",
	"................",
	"................",
	"................",
	"................",
	".....oooo.......",
	"....obbbbo......",
	"...obbbbbbo.....",
	"...obsbbbbo.....",
	"...obbbbbbo.....",
	"....oooooo......",
	"................",
	"................",
	"................",
	"................",
	"................",
}

var itemCoin = []string{
	"................",
	"................",
	"................",
	"................",
	"......oooo......",
	".....obbbbo.....",
	"....obbssbbo....",
	"....obsbbsbo....",
	"....obsbbsbo....",
	"....obbssbbo....",
	".....obbbbo.....",
	"......oooo......",
	"................",
	"................",
	"................",
	"................",
}

var itemBook = []string{
	"................",
	"................",
	"................",
	"....oooooooo....",
	"....obbbbbbo....",
	"....obsssbbo....",
	"....obbbbbbo....",
	"....obsssbbo....",
	"....obbbbbbo....",
	"....obsssbbo....",
	"....obbbbbbo....",
	"....oooooooo....",
	"................",
	"................",
	"................",
	"................",
}

var itemTrinket = []string{
	"................",
	"................",
	"................",
	"......oooo......",
	".....obbbbo.....",
	"....obbssbbo....",
	"...obbsbbsbbo...",
	"....obbssbbo....",
	".....obbbbo.....",
	"......obbo......",
	".......oo.......",
	"................",
	"................",
	"................",
	"................",
	"................",
}

var itemHide = []string{
	"................",
	"................",
	"................",
	"...oo......oo...",
	"..obbooooooobbo.",
	"..obbbbbbbbbbbo.",
	"..obbsbbbbsbbbo.",
	"..obbbbbbbbbbbo.",
	"..obbbbbbbbbbbo.",
	"...obbooooobbo..",
	"...oo......oo...",
	"................",
	"................",
	"................",
	"................",
	"................",
}
