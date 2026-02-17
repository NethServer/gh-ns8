package module_release

import (
	"fmt"
	"strings"

	"github.com/NethServer/gh-ns8/internal/github"
)

// ANSI color codes
const (
	ColorReset   = "\033[0m"
	ColorBold    = "\033[1m"
	ColorYellow  = "\033[33m"
	ColorCyan    = "\033[36m"
	ColorMagenta = "\033[35m"
	ColorGreen   = "\033[32m"
)

// Status emojis
const (
	EmojiOpenIssue   = "🟢"
	EmojiClosedIssue = "🟣"
	EmojiInProgress  = "🚧"
	EmojiTesting     = "🔨"
	EmojiVerified    = "✅"
)

// IssueInfo holds display information about an issue
type IssueInfo struct {
	Number       int
	Status       string // Open/Closed emoji
	Progress     string // Progress emoji
	Labels       string // Filtered labels (without testing/verified)
	RefCount     int    // Number of PRs referencing this issue
	ParentNumber int    // Parent issue number (0 if none)
	Children     []int  // Child issue numbers
}

// CheckSummary holds all information for the check command display
type CheckSummary struct {
	UnlinkedPRs    []string
	TranslationPRs []string
	OrphanCommits  []string
	Issues         map[int]*IssueInfo
	IssuesRepo     string
}

// NewCheckSummary creates a new CheckSummary
func NewCheckSummary(issuesRepo string) *CheckSummary {
	return &CheckSummary{
		Issues:     make(map[int]*IssueInfo),
		IssuesRepo: issuesRepo,
	}
}

// ProcessIssue processes an issue and adds it to the summary
func (cs *CheckSummary) ProcessIssue(client *github.Client, issueNumber int) error {
	// Check if already processed
	if info, exists := cs.Issues[issueNumber]; exists {
		info.RefCount++
		return nil
	}

	// Get issue details
	issue, err := client.GetIssue(cs.IssuesRepo, issueNumber)
	if err != nil {
		return fmt.Errorf("failed to get issue %d: %w", issueNumber, err)
	}

	// Create issue info
	info := &IssueInfo{
		Number:   issueNumber,
		RefCount: 1,
	}

	// Set status
	if issue.State == "CLOSED" || issue.State == "closed" {
		info.Status = EmojiClosedIssue
	} else {
		info.Status = EmojiOpenIssue
	}

	// Extract labels and progress
	var labelNames []string
	hasVerified := false
	hasTesting := false

	for _, label := range issue.Labels {
		if label.Name == "verified" {
			hasVerified = true
		} else if label.Name == "testing" {
			hasTesting = true
		} else {
			labelNames = append(labelNames, label.Name)
		}
	}

	info.Labels = strings.Join(labelNames, " ")

	// Set progress status
	if hasVerified {
		info.Progress = EmojiVerified
	} else if hasTesting {
		info.Progress = EmojiTesting
	} else {
		info.Progress = EmojiInProgress
	}

	// Check for parent issue
	parentNum, err := client.GetParentIssueNumber(cs.IssuesRepo, issueNumber)
	if err == nil && parentNum > 0 {
		info.ParentNumber = parentNum
		// Recursively process parent if not already processed
		if _, exists := cs.Issues[parentNum]; !exists {
			if err := cs.ProcessIssue(client, parentNum); err == nil {
				// Add this issue as a child of parent
				cs.Issues[parentNum].Children = append(cs.Issues[parentNum].Children, issueNumber)
			}
		} else {
			// Parent already exists, just add as child
			cs.Issues[parentNum].Children = append(cs.Issues[parentNum].Children, issueNumber)
		}
	}

	cs.Issues[issueNumber] = info
	return nil
}

// Display prints the check summary
func (cs *CheckSummary) Display() {
	fmt.Println("Summary:")
	fmt.Println("--------")

	// Unlinked PRs
	if len(cs.UnlinkedPRs) > 0 {
		fmt.Printf("%sPRs without linked issues:%s\n", ColorYellow, ColorReset)
		for _, pr := range cs.UnlinkedPRs {
			fmt.Println(pr)
		}
		fmt.Println()
	}

	// Translation PRs
	if len(cs.TranslationPRs) > 0 {
		fmt.Printf("%sTranslation PRs:%s\n", ColorCyan, ColorReset)
		for _, pr := range cs.TranslationPRs {
			fmt.Println(pr)
		}
		fmt.Println()
	}

	// Orphan commits
	if len(cs.OrphanCommits) > 0 {
		fmt.Printf("%sCommits outside PRs:%s\n", ColorMagenta, ColorReset)
		for _, commit := range cs.OrphanCommits {
			fmt.Println(commit)
		}
		fmt.Println()
	}

	// Issues
	fmt.Printf("%sIssues:%s\n", ColorBold, ColorReset)

	// Display parent issues with children first
	for _, info := range cs.Issues {
		if info.ParentNumber == 0 && len(info.Children) > 0 {
			cs.displayIssue(info, 0)
		}
	}

	// Display standalone issues (no parent, no children)
	for _, info := range cs.Issues {
		if info.ParentNumber == 0 && len(info.Children) == 0 {
			cs.displayIssue(info, 0)
		}
	}

	// Check if all verified
	allVerified := true
	for _, info := range cs.Issues {
		// Skip parent issues with children (check children instead)
		if len(info.Children) > 0 {
			for _, childNum := range info.Children {
				if cs.Issues[childNum].Progress != EmojiVerified {
					allVerified = false
					break
				}
			}
		} else if info.Progress != EmojiVerified {
			allVerified = false
			break
		}
	}

	if len(cs.UnlinkedPRs) == 0 && allVerified {
		fmt.Println()
		fmt.Printf("%s✅ All checks passed! Ready to release.%s\n", ColorGreen, ColorReset)
	}

	// Legend
	fmt.Println("---")
	fmt.Printf("Issue status:    %s Open    %s Closed\n", EmojiOpenIssue, EmojiClosedIssue)
	fmt.Printf("Progress status: %s In Progress    %s Testing    %s Verified\n", EmojiInProgress, EmojiTesting, EmojiVerified)
}

// displayIssue displays a single issue (and recursively its children)
func (cs *CheckSummary) displayIssue(info *IssueInfo, indent int) {
	prefix := strings.Repeat("  ", indent)
	if indent > 0 {
		prefix = "└─"
	}

	issueURL := fmt.Sprintf("https://github.com/%s/issues/%d", cs.IssuesRepo, info.Number)
	fmt.Printf("%-6s%s %s %-45s (%d) %s\n",
		prefix,
		info.Status,
		info.Progress,
		issueURL,
		info.RefCount,
		info.Labels)

	// Display children
	for _, childNum := range info.Children {
		if childInfo, exists := cs.Issues[childNum]; exists {
			cs.displayChildIssue(childInfo)
		}
	}
}

// displayChildIssue displays a child issue with proper indentation
func (cs *CheckSummary) displayChildIssue(info *IssueInfo) {
	issueURL := fmt.Sprintf("https://github.com/%s/issues/%d", cs.IssuesRepo, info.Number)
	fmt.Printf("%-2s%-2s %s %-45s (%d) %s\n",
		"└─",
		info.Status,
		info.Progress,
		issueURL,
		info.RefCount,
		info.Labels)
}
