package module_release

import (
	"fmt"
	"strconv"
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
	issueOrder     []int
}

type issueProvider interface {
	GetIssue(repo string, number int) (*github.Issue, error)
	GetParentIssueNumber(repo string, issueNumber int) (int, error)
}

// NewCheckSummary creates a new CheckSummary
func NewCheckSummary(issuesRepo string) *CheckSummary {
	return &CheckSummary{
		Issues:     make(map[int]*IssueInfo),
		IssuesRepo: issuesRepo,
	}
}

// ProcessIssue processes an issue and adds it to the summary
func (cs *CheckSummary) ProcessIssue(client issueProvider, issueNumber int) error {
	// Check if already processed
	if info, exists := cs.Issues[issueNumber]; exists {
		info.RefCount++
		return nil
	}

	info, err := cs.loadIssueInfo(client, issueNumber, 1)
	if err != nil {
		return err
	}

	// Check for parent issue
	parentNum, err := client.GetParentIssueNumber(cs.IssuesRepo, issueNumber)
	if err == nil && parentNum > 0 {
		parentInfo, exists := cs.Issues[parentNum]
		if !exists {
			parentInfo, err = cs.loadIssueInfo(client, parentNum, 0)
			if err == nil {
				cs.Issues[parentNum] = parentInfo
				cs.rememberTopLevelIssue(parentNum)
			}
		}
		if parentInfo != nil {
			info.ParentNumber = parentNum
			parentInfo.Children = append(parentInfo.Children, issueNumber)
		} else {
			cs.rememberTopLevelIssue(issueNumber)
		}
	} else {
		cs.rememberTopLevelIssue(issueNumber)
	}

	cs.Issues[issueNumber] = info
	return nil
}

func (cs *CheckSummary) loadIssueInfo(client issueProvider, issueNumber, refCount int) (*IssueInfo, error) {
	issue, err := client.GetIssue(cs.IssuesRepo, issueNumber)
	if err != nil {
		return nil, fmt.Errorf("failed to get issue %d: %w", issueNumber, err)
	}

	info := &IssueInfo{
		Number:   issueNumber,
		RefCount: refCount,
	}

	if issue.State == "CLOSED" || issue.State == "closed" {
		info.Status = EmojiClosedIssue
	} else {
		info.Status = EmojiOpenIssue
	}

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

	switch {
	case hasVerified:
		info.Progress = EmojiVerified
	case hasTesting:
		info.Progress = EmojiTesting
	default:
		info.Progress = EmojiInProgress
	}

	return info, nil
}

func (cs *CheckSummary) rememberTopLevelIssue(issueNumber int) {
	for _, current := range cs.issueOrder {
		if current == issueNumber {
			return
		}
	}
	cs.issueOrder = append(cs.issueOrder, issueNumber)
}

func (cs *CheckSummary) orderedTopLevelIssues() []*IssueInfo {
	topLevel := make([]int, 0, len(cs.issueOrder))
	seen := make(map[int]bool, len(cs.issueOrder))
	for _, issueNumber := range cs.issueOrder {
		info, exists := cs.Issues[issueNumber]
		if !exists || info.ParentNumber != 0 || seen[issueNumber] {
			continue
		}
		topLevel = append(topLevel, issueNumber)
		seen[issueNumber] = true
	}

	for issueNumber, info := range cs.Issues {
		if info.ParentNumber == 0 && !seen[issueNumber] {
			topLevel = append(topLevel, issueNumber)
		}
	}

	ordered := make([]*IssueInfo, 0, len(topLevel))
	for _, issueNumber := range bashAssocKeyOrder(topLevel) {
		if info, exists := cs.Issues[issueNumber]; exists {
			ordered = append(ordered, info)
		}
	}

	return ordered
}

func bashAssocKeyOrder(keys []int) []int {
	table := newBashHashTable()
	for _, key := range keys {
		table.insert(key)
	}
	return table.keys()
}

type bashHashEntry struct {
	key  int
	hash uint32
}

type bashHashTable struct {
	buckets  [][]bashHashEntry
	nentries int
}

func newBashHashTable() *bashHashTable {
	return &bashHashTable{
		buckets: make([][]bashHashEntry, 1024),
	}
}

func (t *bashHashTable) insert(key int) {
	if t.nentries >= len(t.buckets)*2 {
		t.rehash(len(t.buckets) * 4)
	}

	hash := bashHashString(strconv.Itoa(key))
	bucket := int(hash & uint32(len(t.buckets)-1))

	for _, entry := range t.buckets[bucket] {
		if entry.key == key {
			return
		}
	}

	t.buckets[bucket] = append([]bashHashEntry{{key: key, hash: hash}}, t.buckets[bucket]...)
	t.nentries++
}

func (t *bashHashTable) rehash(size int) {
	buckets := make([][]bashHashEntry, size)
	for _, bucketEntries := range t.buckets {
		for _, entry := range bucketEntries {
			bucket := int(entry.hash & uint32(size-1))
			buckets[bucket] = append([]bashHashEntry{entry}, buckets[bucket]...)
		}
	}
	t.buckets = buckets
}

func (t *bashHashTable) keys() []int {
	keys := make([]int, 0, t.nentries)
	for _, bucketEntries := range t.buckets {
		for _, entry := range bucketEntries {
			keys = append(keys, entry.key)
		}
	}
	return keys
}

func bashHashString(key string) uint32 {
	var hash uint32 = 2166136261
	for i := 0; i < len(key); i++ {
		hash += (hash << 1) + (hash << 4) + (hash << 7) + (hash << 8) + (hash << 24)
		hash ^= uint32(key[i])
	}
	return hash
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

	for _, info := range cs.orderedTopLevelIssues() {
		cs.displayIssue(info)
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

// displayIssue displays a single top-level issue and its direct children.
func (cs *CheckSummary) displayIssue(info *IssueInfo) {
	issueURL := fmt.Sprintf("https://github.com/%s/issues/%d", cs.IssuesRepo, info.Number)
	fmt.Printf("%s   %s %s (%d) %s\n",
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
	fmt.Printf("└─%s %s %s (%d) %s\n",
		info.Status,
		info.Progress,
		issueURL,
		info.RefCount,
		info.Labels)
}
