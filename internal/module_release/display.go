package module_release

import (
	"fmt"
	"sort"
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
	EmojiOpenPR      = "🟩"
	EmojiMergedPR    = "🟪"
	EmojiClosedPR    = "⬛"
	EmojiRenovate    = "🤖"
	EmojiTranslation = "🌐"
	EmojiMerged      = "🔀"
)

// Open PR mergeability values
const (
	PRMergeable = "mergeable"
	PRBlocked   = "blocked"
	PRUnknown   = "unknown"
)

// maxTitleLength caps the displayed title length for issues and PRs.
const maxTitleLength = 72

// hyperlink wraps text in an OSC 8 terminal hyperlink pointing at url.
func hyperlink(url, text string) string {
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, text)
}

// truncateTitle shortens a title to maxTitleLength runes, adding an ellipsis
// when the title is cut.
func truncateTitle(title string) string {
	runes := []rune(title)
	if len(runes) <= maxTitleLength {
		return title
	}
	return string(runes[:maxTitleLength-1]) + "…"
}

// titleLink renders "#number truncated-title" as an OSC 8 hyperlink to url,
// falling back to just "#number" when no title is available.
func titleLink(number int, title, url string) string {
	prefix := fmt.Sprintf("#%d", number)
	text := truncateTitle(title)
	if text != "" {
		text = prefix + " " + text
	} else {
		text = prefix
	}
	return hyperlink(url, text)
}

// IssueInfo holds display information about an issue
type IssueInfo struct {
	Number       int
	Title        string // Issue title
	Status       string // Open/Closed emoji
	Progress     string // Progress emoji
	Labels       string // Filtered labels (without testing/verified)
	RefCount     int    // Number of PRs referencing this issue
	ParentNumber int    // Parent issue number (0 if none)
	Children     []int  // Child issue numbers
	LinkedPRs    []PRInfo
}

// PRCategory identifies the display bucket for a PR.
type PRCategory int

const (
	PRCategoryRenovate PRCategory = iota
	PRCategoryTranslation
	PRCategoryGeneric
	PRCategoryMerged
)

// PRInfo holds display information about a pull request.
type PRInfo struct {
	Number       int
	Category     PRCategory
	URL          string
	Title        string
	Status       string
	Progress     string
	Mergeability string
	Labels       string
}

// CheckSummary holds all information for the check command display
type CheckSummary struct {
	RenovatePRs    []PRInfo
	TranslationPRs []PRInfo
	GenericPRs     []PRInfo
	MergedPRs      []PRInfo
	OpenWeblatePRs []string
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

// AddPullRequest adds a pull request to the requested display category.
func (cs *CheckSummary) AddPullRequest(repo string, pr *github.PullRequest, category PRCategory) {
	info := newPRInfo(repo, pr, category)
	switch category {
	case PRCategoryRenovate:
		cs.RenovatePRs = append(cs.RenovatePRs, info)
	case PRCategoryTranslation:
		cs.TranslationPRs = append(cs.TranslationPRs, info)
	case PRCategoryGeneric:
		cs.GenericPRs = append(cs.GenericPRs, info)
	default:
		cs.MergedPRs = append(cs.MergedPRs, info)
	}
}

func newPRInfo(repo string, pr *github.PullRequest, category PRCategory) PRInfo {
	return PRInfo{
		Number:       pr.Number,
		Category:     category,
		URL:          pullRequestURL(repo, pr),
		Title:        pr.Title,
		Status:       pullRequestStatus(pr),
		Progress:     pullRequestProgress(category),
		Mergeability: pullRequestMergeability(pr),
		Labels:       pullRequestLabels(pr),
	}
}

func pullRequestURL(repo string, pr *github.PullRequest) string {
	if pr.HTMLURL != "" {
		return pr.HTMLURL
	}
	return fmt.Sprintf("https://github.com/%s/pull/%d", repo, pr.Number)
}

func pullRequestStatus(pr *github.PullRequest) string {
	if pr.Merged {
		return EmojiMergedPR
	}
	if strings.EqualFold(pr.State, "open") {
		return EmojiOpenPR
	}
	return EmojiClosedPR
}

func pullRequestProgress(category PRCategory) string {
	switch category {
	case PRCategoryRenovate:
		return EmojiRenovate
	case PRCategoryTranslation:
		return EmojiTranslation
	case PRCategoryGeneric:
		return ""
	default:
		return EmojiMerged
	}
}

func pullRequestMergeability(pr *github.PullRequest) string {
	if !strings.EqualFold(pr.State, "open") {
		return ""
	}

	state := strings.ToLower(pr.MergeableState)
	if pr.Draft || state == "draft" {
		return PRBlocked
	}
	if pr.Mergeable == nil || state == "" || state == "unknown" {
		return PRUnknown
	}
	if !*pr.Mergeable {
		return PRBlocked
	}

	switch state {
	case "clean", "has_hooks":
		return PRMergeable
	case "blocked", "dirty", "behind", "unstable":
		return PRBlocked
	default:
		return PRUnknown
	}
}

func pullRequestLabels(pr *github.PullRequest) string {
	var labelNames []string
	for _, label := range pr.Labels {
		if label.Name == "verified" || label.Name == "testing" {
			continue
		}
		labelNames = append(labelNames, label.Name)
	}
	return strings.Join(labelNames, " ")
}

// AddIssuePullRequest records a PR under a linked issue and avoids duplicates.
func (cs *CheckSummary) AddIssuePullRequest(repo string, issueNumber int, pr *github.PullRequest, category PRCategory) {
	info, exists := cs.Issues[issueNumber]
	if !exists {
		return
	}

	prInfo := newPRInfo(repo, pr, category)
	for _, existing := range info.LinkedPRs {
		if existing.Number == prInfo.Number {
			return
		}
	}

	info.LinkedPRs = append(info.LinkedPRs, prInfo)
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
		Title:    issue.Title,
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
	// Open Weblate PRs warning
	if len(cs.OpenWeblatePRs) > 0 {
		fmt.Printf("%s⚠️  Open Weblate PRs detected:%s\n", ColorYellow, ColorReset)
		for _, pr := range cs.OpenWeblatePRs {
			fmt.Println(pr)
		}
		fmt.Println()
	}

	fmt.Println("Summary:")
	fmt.Println("--------")

	cs.displayPullRequests()
	if cs.hasPullRequests() {
		cs.displayPullRequestLegend()
		fmt.Println()
	}

	// Issues
	fmt.Printf("%sIssues:%s\n", ColorBold, ColorReset)

	for _, info := range cs.orderedTopLevelIssues() {
		cs.displayIssue(info)
	}
	cs.displayIssueLegend()

	// Orphan commits
	if len(cs.OrphanCommits) > 0 {
		fmt.Println()
		fmt.Printf("%sCommits outside PRs:%s\n", ColorMagenta, ColorReset)
		for _, commit := range cs.OrphanCommits {
			fmt.Println(commit)
		}
	}

	if len(cs.MergedPRs) == 0 && !cs.hasBlockedOpenPullRequests() && cs.allIssuesVerified() {
		fmt.Println()
		fmt.Printf("%s✅ All checks passed! Ready to release.%s\n", ColorGreen, ColorReset)
	}
}

func (cs *CheckSummary) displayPullRequests() {
	if !cs.hasPullRequests() {
		return
	}

	fmt.Printf("%sPRs:%s\n", ColorBold, ColorReset)
	for _, pr := range cs.orderedPullRequests() {
		displayPullRequest(pr)
	}
}

func displayPullRequest(info PRInfo) {
	details := make([]string, 0, 1)
	if info.Labels != "" {
		details = append(details, info.Labels)
	}

	suffix := ""
	if len(details) > 0 {
		suffix = " " + strings.Join(details, " ")
	}

	fmt.Printf("%s   %s %s%s\n",
		info.Status,
		displayedPullRequestProgress(info),
		titleLink(info.Number, info.Title, info.URL),
		suffix)
}

func displayedPullRequestProgress(info PRInfo) string {
	if info.Status == EmojiOpenPR {
		return ""
	}
	return info.Progress
}

func (cs *CheckSummary) hasPullRequests() bool {
	return len(cs.RenovatePRs) > 0 ||
		len(cs.TranslationPRs) > 0 ||
		len(cs.GenericPRs) > 0 ||
		len(cs.MergedPRs) > 0
}

func (cs *CheckSummary) hasBlockedOpenPullRequests() bool {
	for _, pr := range cs.allPullRequests() {
		if pr.Mergeability == PRBlocked {
			return true
		}
	}
	return false
}

func (cs *CheckSummary) allPullRequests() []PRInfo {
	prs := make([]PRInfo, 0,
		len(cs.RenovatePRs)+
			len(cs.TranslationPRs)+
			len(cs.GenericPRs)+
			len(cs.MergedPRs))
	prs = append(prs, cs.allTopLevelPullRequests()...)
	for _, issue := range cs.Issues {
		prs = append(prs, issue.LinkedPRs...)
	}
	return prs
}

func (cs *CheckSummary) allTopLevelPullRequests() []PRInfo {
	prs := make([]PRInfo, 0,
		len(cs.RenovatePRs)+
			len(cs.TranslationPRs)+
			len(cs.GenericPRs)+
			len(cs.MergedPRs))
	prs = append(prs, cs.RenovatePRs...)
	prs = append(prs, cs.TranslationPRs...)
	prs = append(prs, cs.GenericPRs...)
	prs = append(prs, cs.MergedPRs...)
	return prs
}

func (cs *CheckSummary) orderedPullRequests() []PRInfo {
	return orderedPullRequestInfos(cs.allTopLevelPullRequests())
}

func orderedPullRequestInfos(prs []PRInfo) []PRInfo {
	ordered := make([]PRInfo, 0,
		len(prs))
	for _, status := range []string{EmojiOpenPR, EmojiMergedPR, EmojiClosedPR} {
		for _, category := range []PRCategory{
			PRCategoryRenovate,
			PRCategoryTranslation,
			PRCategoryGeneric,
			PRCategoryMerged,
		} {
			for _, pr := range sortPullRequestGroup(prs) {
				if pr.Status == status && pr.Category == category {
					ordered = append(ordered, pr)
				}
			}
		}
	}

	return ordered
}

func sortPullRequestGroup(prs []PRInfo) []PRInfo {
	ordered := append([]PRInfo(nil), prs...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].Number < ordered[j].Number
	})
	return ordered
}

func (cs *CheckSummary) displayPullRequestLegend() {
	fmt.Println("---")
	fmt.Printf("PR status:       %s Open    %s Merged    %s Closed\n", EmojiOpenPR, EmojiMergedPR, EmojiClosedPR)
	fmt.Printf("PR type:         %s Renovate    %s Translation    %s Merged\n", EmojiRenovate, EmojiTranslation, EmojiMerged)
}

func (cs *CheckSummary) displayIssueLegend() {
	fmt.Println("---")
	fmt.Printf("Issue status:    %s Open    %s Closed\n", EmojiOpenIssue, EmojiClosedIssue)
	fmt.Printf("Progress status: %s In Progress    %s Testing    %s Verified\n", EmojiInProgress, EmojiTesting, EmojiVerified)
}

func (cs *CheckSummary) allIssuesVerified() bool {
	for _, info := range cs.Issues {
		// Skip parent issues with children (check children instead)
		if len(info.Children) > 0 {
			for _, childNum := range info.Children {
				if cs.Issues[childNum].Progress != EmojiVerified {
					return false
				}
			}
		} else if info.Progress != EmojiVerified {
			return false
		}
	}

	return true
}

// displayIssue displays a single top-level issue and its direct children.
func (cs *CheckSummary) displayIssue(info *IssueInfo) {
	issueURL := fmt.Sprintf("https://github.com/%s/issues/%d", cs.IssuesRepo, info.Number)
	connector := "  "
	if len(info.Children) == 0 {
		connector = "──"
	}
	fmt.Printf("%s%s %s %s\n",
		info.Status,
		connector,
		info.Progress,
		titleLink(info.Number, info.Title, issueURL))

	for _, pr := range orderedPullRequestInfos(info.LinkedPRs) {
		displayNestedPullRequest(pr)
	}

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
	fmt.Printf("└─%s %s %s\n",
		info.Status,
		info.Progress,
		titleLink(info.Number, info.Title, issueURL))

	for _, pr := range orderedPullRequestInfos(info.LinkedPRs) {
		displayNestedPullRequest(pr)
	}
}

func displayNestedPullRequest(info PRInfo) {
	details := make([]string, 0, 1)
	if info.Labels != "" {
		details = append(details, info.Labels)
	}

	suffix := ""
	if len(details) > 0 {
		suffix = " " + strings.Join(details, " ")
	}

	fmt.Printf("        • %s %s%s\n",
		info.Status,
		titleLink(info.Number, info.Title, info.URL),
		suffix)
}
