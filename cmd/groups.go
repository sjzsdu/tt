package cmd

import "github.com/spf13/cobra"

const (
	groupAgentsID    = "agents"
	groupCodingID    = "coding"
	groupContentID   = "content"
	groupWebTestID   = "webtest"
	groupUIID        = "ui"
	groupKnowledgeID = "knowledge"
	groupMiscID      = "misc"
)

var rootCommandGroups = []*cobra.Group{
	{ID: groupAgentsID, Title: "Agent 与智能体协作:"},
	{ID: groupCodingID, Title: "编码工作流 (Formula):"},
	{ID: groupContentID, Title: "内容生成 (Slide/视频/图片):"},
	{ID: groupWebTestID, Title: "Web 测试:"},
	{ID: groupUIID, Title: "本地浏览 UI:"},
	{ID: groupKnowledgeID, Title: "知识与文档:"},
	{ID: groupMiscID, Title: "其他工具:"},
}

// commandGroupByName maps top-level command names to a help group.
// Dynamically registered shortcuts (embedded agents / formula aliases) set
// their GroupID at creation time; this map covers the static commands.
var commandGroupByName = map[string]string{
	// agents
	"agent":  groupAgentsID,
	"team":   groupAgentsID,
	"debate": groupAgentsID,

	// coding workflows
	"coding":               groupCodingID,
	"coding-plan":          groupCodingID,
	"coding-impl":          groupCodingID,
	"bead-coding":          groupCodingID,
	"keep-coding":          groupCodingID,
	"requirement-grooming": groupCodingID,
	"formula":              groupCodingID,

	// content generation
	"slide":           groupContentID,
	"slide-generate":  groupContentID,
	"slides-generate": groupContentID,
	"deck-generate":   groupContentID,
	"slide-writer":    groupContentID,
	"video":           groupContentID,
	"image":           groupContentID,
	"translate":       groupContentID,

	// web testing
	"web-bug-hunt":      groupWebTestID,
	"web-feature-test":  groupWebTestID,
	"web-targeted-test": groupWebTestID,

	// local browsing UIs
	"web":          groupUIID,
	"share":        groupUIID,
	"json":         groupUIID,
	"markdown":     groupUIID,
	"conversation": groupUIID,
	"beads":        groupUIID,

	// knowledge & docs
	"docs":       groupKnowledgeID,
	"repo2skill": groupKnowledgeID,
	"cmd2skill":  groupKnowledgeID,

	// misc
	"computer":   groupMiscID,
	"config":     groupMiscID,
	"mirror":     groupMiscID,
	"onboard":    groupMiscID,
	"completion": groupMiscID,
	"help":       groupMiscID,
	"version":    groupMiscID,
}

// formulaGroupByName assigns newly registered formula alias shortcuts to a
// help group based on the underlying formula name.
var formulaGroupByName = map[string]string{
	"coding":                groupCodingID,
	"coding-implementation": groupCodingID,
	"coding-requirement":    groupCodingID,
	"bead-coding":           groupCodingID,
	"keep-coding":           groupCodingID,
	"requirement-grooming":  groupCodingID,
	"slide-generate":        groupContentID,
	"web-bug-hunt":          groupWebTestID,
	"web-feature-test":      groupWebTestID,
}

func formulaGroupID(formulaName string) string {
	if id, ok := formulaGroupByName[formulaName]; ok {
		return id
	}
	return groupMiscID
}

// applyCommandGroups registers the help groups on the root command and
// assigns every visible subcommand to a group so `tt --help` renders
// categorized sections instead of one flat list.
func applyCommandGroups() {
	rootCmd.AddGroup(rootCommandGroups...)
	for _, c := range rootCmd.Commands() {
		// Static mapping wins so dynamically registered shortcuts
		// (e.g. slide-writer) can still be regrouped by name.
		if gid, ok := commandGroupByName[c.Name()]; ok {
			c.GroupID = gid
			continue
		}
		if c.GroupID == "" {
			c.GroupID = groupMiscID
		}
	}
}
