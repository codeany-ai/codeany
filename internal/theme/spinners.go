package theme

import (
	"math/rand"
	"time"
)

// SpinnerVerbs contains whimsical activity descriptions shown during queries
var SpinnerVerbs = []string{
	"Thinking",
	"Reasoning",
	"Analyzing",
	"Pondering",
	"Computing",
	"Processing",
	"Generating",
	"Crafting",
	"Assembling",
	"Composing",
	"Architecting",
	"Synthesizing",
	"Evaluating",
	"Considering",
	"Formulating",
	"Deliberating",
	"Contemplating",
	"Brainstorming",
	"Scheming",
	"Plotting",
	"Brewing",
	"Cooking",
	"Baking",
	"Simmering",
	"Marinating",
	"Percolating",
	"Fermenting",
	"Distilling",
	"Kneading",
	"Forging",
	"Welding",
	"Sculpting",
	"Weaving",
	"Tinkering",
	"Wrangling",
	"Crunching",
	"Deciphering",
	"Decoding",
	"Unraveling",
	"Untangling",
	"Manifesting",
	"Conjuring",
	"Summoning",
	"Channeling",
	"Bootstrapping",
	"Compiling",
	"Optimizing",
	"Refactoring",
	"Debugging",
	"Resolving",
	"Orchestrating",
	"Harmonizing",
	"Calibrating",
	"Tuning",
	"Polishing",
}

var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// RandomVerb returns a random spinner verb
func RandomVerb() string {
	return SpinnerVerbs[rng.Intn(len(SpinnerVerbs))]
}

// TurnCompletionVerbs are past-tense verbs shown when a query completes
var TurnCompletionVerbs = []string{
	"Baked", "Brewed", "Churned", "Cogitated", "Cooked",
	"Crunched", "Worked", "Crafted", "Forged", "Polished",
}

// RandomCompletionVerb returns a random completion verb
func RandomCompletionVerb() string {
	return TurnCompletionVerbs[rng.Intn(len(TurnCompletionVerbs))]
}

// ToolVerb returns an appropriate verb for a specific tool
func ToolVerb(toolName string) string {
	switch toolName {
	case "Bash":
		return "Executing"
	case "Read":
		return "Reading"
	case "Write":
		return "Writing"
	case "Edit":
		return "Editing"
	case "Glob":
		return "Searching"
	case "Grep":
		return "Scanning"
	case "WebFetch":
		return "Fetching"
	case "WebSearch":
		return "Searching"
	case "Agent":
		return "Delegating"
	case "TaskCreate", "TaskUpdate":
		return "Organizing"
	default:
		return "Processing"
	}
}
