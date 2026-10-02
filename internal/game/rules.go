package game

import "asyncwargame/internal/rules"

// The module schema lives in package rules; these aliases keep the game
// code readable.
type (
	Module    = rules.Module
	Side      = rules.Side
	Terrain   = rules.Terrain
	Class     = rules.Class
	Contact   = rules.Contact
	EdgeType  = rules.EdgeType
	Crossing  = rules.Crossing
	Factor    = rules.Factor
	Party     = rules.Party
	Result    = rules.Result
	EventDef  = rules.EventDef
	EffectDef = rules.EffectDef
	Range     = rules.Range
	Where     = rules.Where
)
