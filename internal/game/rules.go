package game

import "asyncwargame/internal/rules"

// The module schema lives in package rules; these aliases keep the game
// code readable.
type (
	Module     = rules.Module
	Scale      = rules.Scale
	Side       = rules.Side
	Terrain    = rules.Terrain
	Class      = rules.Class
	Contact    = rules.Contact
	EdgeType   = rules.EdgeType
	Crossing   = rules.Crossing
	Combat     = rules.Combat
	Sight      = rules.Sight
	Pool       = rules.Pool
	Factor     = rules.Factor
	Outcome    = rules.Outcome
	Experience = rules.Experience
	XPLevel    = rules.XPLevel
	Stacking   = rules.Stacking
	OrderSpec  = rules.OrderSpec
	TaskDef    = rules.TaskDef
	Option     = rules.Option
	Trait      = rules.Trait
	EventDef   = rules.EventDef
	EffectDef  = rules.EffectDef
	Range      = rules.Range
	Where      = rules.Where
)

func optionName(opts []Option, id string) string { return rules.OptionName(opts, id) }
