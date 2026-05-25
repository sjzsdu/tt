package cmd

import formulaui "github.com/sjzsdu/tt/internal/formulaui"

type formulaDashboardSnapshot = formulaui.Snapshot
type formulaDashboardStep = formulaui.Step
type formulaStepActivity = formulaui.StepActivity
type formulaDashboardLoop = formulaui.Loop
type formulaDashboardLoopBody = formulaui.LoopBody
type formulaDashboardGate = formulaui.Gate
type formulaDashboardEdge = formulaui.Edge
type formulaDashboardLogEntry = formulaui.LogEntry
type formulaDashboardMessage = formulaui.Message

var buildFormulaDashboardGraph = formulaui.BuildGraph
var cloneFormulaDashboardSnapshot = formulaui.CloneSnapshot
var cloneHumanInputRequest = formulaui.CloneHumanInputRequest
var loopParentStepID = formulaui.LoopParentStepID
var appendStepActivity = formulaui.AppendStepActivity
var cloneStringMap = formulaui.CloneStringMap
var buildFormulaDashboardLoop = formulaui.BuildLoop
var cloneDashboardLoop = formulaui.CloneLoop
