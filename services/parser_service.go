package services

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/google/uuid"
	"performancereport/models"
)

// ParseMonthlyText parses free-form monthly work text or markdown into structured tasks.
func ParseMonthlyText(text string) *models.ParseResult {
	rawLines := splitLines(text)
	result := &models.ParseResult{}

	var lines []string
	for _, l := range rawLines {
		trimmed := strings.TrimSpace(l)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	var currentProject string
	var currentTask *models.ParsedTask
	var currentSection string = "desc" // "desc", "issue", "activities", "solution", "outcome"
	var descBuf []string
	var projects []string
	projectSet := make(map[string]bool)

	var inTOC bool
	var inConclusion bool
	var conclusionLines []string

	flushTask := func() {
		if currentTask != nil {
			if len(descBuf) > 0 {
				combined := strings.Join(descBuf, "\n")
				if currentTask.DetailedDescription == "" {
					currentTask.DetailedDescription = combined
				} else {
					currentTask.DetailedDescription += "\n" + combined
				}
			}
			if currentTask.ActivitiesPerformed == "" && currentTask.DetailedDescription != "" {
				currentTask.ActivitiesPerformed = currentTask.DetailedDescription
			}
			if currentTask.DetailedDescription == "" && currentTask.ActivitiesPerformed != "" {
				currentTask.DetailedDescription = currentTask.ActivitiesPerformed
			}
			currentTask.Title = cleanDisplayTitle(currentTask.Title)
			if currentTask.Title != "" {
				result.Tasks = append(result.Tasks, *currentTask)
			}
			currentTask = nil
			descBuf = nil
			currentSection = "desc"
		}
	}

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// 1. Skip Markdown Horizontal Rules: ---, ***, ___
		if isHorizontalRule(line) {
			continue
		}

		// 2. Detect Table of Contents block
		if isTOCHeader(line) {
			inTOC = true
			continue
		}
		if inTOC {
			if isConclusionHeading(line) {
				inTOC = false
				continue
			}
			if isTOCLine(line) {
				continue
			}
			// If this line is a genuine project heading and immediately followed by task & subsection,
			// TOC is finished and the actual report body has begun!
			if isProj, _ := checkProjectHeading(line, lines, i); isProj && isNextLineSubsection(lines, i) {
				inTOC = false
				i-- // Reprocess current line as body content
				continue
			}
			continue
		}

		// 3. Skip overall document title (e.g. # Monthly Performance Report - October 2026)
		if isDocumentTitle(line) {
			continue
		}

		// 3b. Skip "General" heading if entered or present
		if strings.EqualFold(strings.TrimSpace(cleanMarkdownTokens(line)), "general") {
			continue
		}

		// 3c. Detect Employee details if pasted in text
		lowerLine := strings.ToLower(cleanMarkdownTokens(line))
		if strings.HasPrefix(lowerLine, "employee name:") || strings.HasPrefix(lowerLine, "name:") {
			val := strings.TrimSpace(line[strings.Index(line, ":")+1:])
			if val != "" {
				result.EmployeeName = val
				if prof, err := GetProfile(); err == nil {
					prof.EmployeeName = val
					_ = SaveProfile(prof)
				}
			}
			continue
		}
		if strings.HasPrefix(lowerLine, "employee code:") || strings.HasPrefix(lowerLine, "code:") || strings.HasPrefix(lowerLine, "emp code:") {
			val := strings.TrimSpace(line[strings.Index(line, ":")+1:])
			if val != "" {
				result.EmployeeCode = val
				if prof, err := GetProfile(); err == nil {
					prof.EmployeeCode = val
					_ = SaveProfile(prof)
				}
			}
			continue
		}
		if strings.HasPrefix(lowerLine, "appraiser name:") || strings.HasPrefix(lowerLine, "appraiser:") {
			val := strings.TrimSpace(line[strings.Index(line, ":")+1:])
			if idxRemark := strings.Index(strings.ToLower(val), "you can"); idxRemark != -1 {
				val = strings.TrimSpace(val[:idxRemark])
			}
			if val != "" {
				result.AppraiserName = val
				if prof, err := GetProfile(); err == nil {
					prof.AppraiserName = val
					_ = SaveProfile(prof)
				}
			}
			continue
		}

		// 3d. Detect Project: or Projects: list (e.g. "Project: ERA Live 2.0, ERA Live 3.0, EMACloudContentManager & EMASECUREBROWSER")
		if strings.HasPrefix(lowerLine, "project:") || strings.HasPrefix(lowerLine, "projects:") || strings.HasPrefix(lowerLine, "module:") || strings.HasPrefix(lowerLine, "modules:") {
			projs := splitProjectList(line)
			if len(projs) > 0 {
				flushTask()
				for _, p := range projs {
					if !projectSet[p] {
						projects = append(projects, p)
						projectSet[p] = true
					}
				}
				currentProject = projs[0]
				currentSection = "desc"
				continue
			}
		}

		// 4. Detect Conclusion section
		if isConclusionHeading(line) {
			flushTask()
			inConclusion = true
			continue
		}
		if inConclusion {
			if isLevel1Heading(line) || isProjectHeadingFast(line) {
				inConclusion = false
			} else {
				conclusionLines = append(conclusionLines, stripMarkdownFormatting(line))
				continue
			}
		}

		// 5. Detect Subsections: Issue Identified, Activities Performed, Solution Implemented, Outcome
		if isSub, subType, inline := isSubsectionHeading(line); isSub {
			currentSection = subType
			if inline != "" {
				cleaned := stripMarkdownFormatting(inline)
				appendSectionContent(currentTask, subType, cleaned)
			}
			continue
		}

		// 6. Detect Project Heading
		if isProj, projName := checkProjectHeading(line, lines, i); isProj && IsValidProjectName(projName) {
			flushTask()
			currentProject = projName
			if !projectSet[currentProject] {
				projects = append(projects, currentProject)
				projectSet[currentProject] = true
			}
			currentSection = "desc"
			continue
		}

		// 7. Detect Task Title
		if isTsk, taskTitle := checkTaskTitle(line, lines, i, currentProject); isTsk {
			flushTask()
			currentTask = &models.ParsedTask{
				ID:      uuid.New().String(),
				Project: currentProject,
				Title:   taskTitle,
			}
			currentSection = "desc"
			continue
		}

		// 8. Content line (bullets, descriptions, etc.)
		cleanedContent := stripMarkdownFormatting(line)
		if currentTask == nil {
			currentTask = &models.ParsedTask{
				ID:              uuid.New().String(),
				Project:         currentProject,
				Title:           cleanDisplayTitle(cleanedContent),
				IsAmbiguous:     true,
				AmbiguityReason: "Task title detected from description paragraph",
			}
			currentSection = "desc"
		} else {
			if currentSection == "desc" {
				descBuf = append(descBuf, cleanedContent)
			} else {
				appendSectionContent(currentTask, currentSection, cleanedContent)
			}
		}
	}
	flushTask()

	filteredProjects := make([]string, 0, len(projects))
	seenProj := make(map[string]bool)
	for _, p := range projects {
		pClean := cleanProjectName(p)
		if pClean != "" && IsValidProjectName(pClean) && !seenProj[strings.ToLower(pClean)] {
			filteredProjects = append(filteredProjects, pClean)
			seenProj[strings.ToLower(pClean)] = true
		}
	}
	result.Projects = filteredProjects
	if len(conclusionLines) > 0 {
		result.Conclusion = strings.Join(conclusionLines, "\n\n")
	}

	for i := range result.Tasks {
		if result.Tasks[i].Project == "" {
			result.Tasks[i].IsAmbiguous = true
			result.Tasks[i].AmbiguityReason = "No project heading found above this task"
		}
		if result.Tasks[i].ShortDescription == "" {
			s := result.Tasks[i].DetailedDescription
			if s == "" {
				s = result.Tasks[i].ActivitiesPerformed
			}
			if idx := strings.IndexAny(s, ".!?"); idx != -1 && idx < 120 {
				result.Tasks[i].ShortDescription = s[:idx+1]
			} else if len(s) > 120 {
				result.Tasks[i].ShortDescription = s[:120] + "..."
			} else {
				result.Tasks[i].ShortDescription = s
			}
		}
	}
	return result
}

func appendSectionContent(task *models.ParsedTask, section, text string) {
	if task == nil || text == "" {
		return
	}
	switch section {
	case "issue":
		if task.IssueIdentified == "" {
			task.IssueIdentified = text
		} else {
			task.IssueIdentified += "\n" + text
		}
	case "activities":
		if task.ActivitiesPerformed == "" {
			task.ActivitiesPerformed = text
		} else {
			task.ActivitiesPerformed += "\n" + text
		}
	case "solution":
		if task.SolutionImplemented == "" {
			task.SolutionImplemented = text
		} else {
			task.SolutionImplemented += "\n" + text
		}
	case "outcome":
		if task.Outcome == "" {
			task.Outcome = text
		} else {
			task.Outcome += "\n" + text
		}
	default:
		if task.DetailedDescription == "" {
			task.DetailedDescription = text
		} else {
			task.DetailedDescription += "\n" + text
		}
	}
}

func isHorizontalRule(s string) bool {
	t := strings.TrimSpace(s)
	if t == "---" || t == "***" || t == "___" || strings.HasPrefix(t, "----") || strings.HasPrefix(t, "****") {
		return true
	}
	return false
}

func isTOCHeader(s string) bool {
	lower := strings.ToLower(cleanMarkdownTokens(s))
	return lower == "table of contents" || lower == "table of content" || lower == "index" || strings.HasPrefix(lower, "table of contents")
}

func isTOCLine(s string) bool {
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "- [") || strings.HasPrefix(t, "* [") || strings.HasPrefix(t, "[") {
		return true
	}
	re := regexp.MustCompile(`^\d+[\.\)]\s*\[`)
	return re.MatchString(t)
}

func isDocumentTitle(s string) bool {
	lower := strings.ToLower(cleanMarkdownTokens(s))
	return strings.HasPrefix(lower, "monthly performance report") || strings.HasPrefix(lower, "performance report")
}

func isConclusionHeading(s string) bool {
	lower := strings.ToLower(cleanMarkdownTokens(s))
	return lower == "conclusion" || strings.HasPrefix(lower, "conclusion:") || strings.HasPrefix(lower, "conclusion -")
}

func isLevel1Heading(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "# ")
}

// splitProjectList splits multiple projects from strings like "Project: ERA Live 2.0, ERA Live 3.0, EMACloudContentManager & EMASECUREBROWSER"
func splitProjectList(s string) []string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "Project:")
	s = strings.TrimPrefix(s, "project:")
	s = strings.TrimPrefix(s, "Projects:")
	s = strings.TrimPrefix(s, "projects:")
	s = strings.TrimPrefix(s, "Module:")
	s = strings.TrimPrefix(s, "module:")
	s = strings.TrimPrefix(s, "Modules:")
	s = strings.TrimPrefix(s, "modules:")
	s = strings.ReplaceAll(s, " & ", ", ")
	s = strings.ReplaceAll(s, " and ", ", ")
	s = strings.ReplaceAll(s, ";", ",")
	parts := strings.Split(s, ",")
	res := []string{}
	seen := make(map[string]bool)
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		trimmed = cleanProjectName(trimmed)
		if trimmed != "" && IsValidProjectName(trimmed) && !strings.EqualFold(trimmed, "General") && !seen[strings.ToLower(trimmed)] {
			res = append(res, trimmed)
			seen[strings.ToLower(trimmed)] = true
		}
	}
	return res
}

// IsValidProjectName verifies if a candidate string is a valid project name.
func IsValidProjectName(s string) bool {
	t := strings.TrimSpace(cleanMarkdownTokens(s))
	if t == "" || len(t) < 2 || len(t) > 60 {
		return false
	}
	if isHorizontalRule(t) || strings.Trim(t, "-*_:=~ ") == "" {
		return false
	}
	lower := strings.ToLower(t)
	lower = strings.TrimSpace(strings.TrimRight(lower, ":-–— "))
	if lower == "" || lower == "general" {
		return false
	}
	invalidKeywords := []string{
		"issue identified", "issues identified", "issue", "issues",
		"activity performed", "activities performed", "activity", "activities",
		"solution implemented", "solutions implemented", "solution", "solutions",
		"outcome", "outcomes", "result", "results", "output",
		"conclusion", "conclusions", "summary",
		"table of contents", "table of content", "index", "toc",
		"monthly performance report", "performance report", "report",
		"employee name", "employee code", "appraiser name", "department",
		"part a", "part b", "part c", "part d",
		"general",
		"---", "===", "***", "___",
	}
	for _, inv := range invalidKeywords {
		if lower == inv || strings.HasPrefix(lower, inv+":") || strings.HasPrefix(lower, inv+" -") || strings.HasPrefix(lower, inv+" –") {
			return false
		}
	}
	return true
}

func isNextLineSubsection(lines []string, idx int) bool {
	for k := idx + 1; k < len(lines) && k <= idx+3; k++ {
		if isSub, _, _ := isSubsectionHeading(lines[k]); isSub {
			return true
		}
	}
	return false
}

func isProjectHeadingFast(s string) bool {
	cleaned := cleanMarkdownTokens(s)
	if !IsValidProjectName(cleaned) {
		return false
	}
	rePart := regexp.MustCompile(`^Part\s+[A-Za-z]\s*[–\-—:]\s*`)
	if rePart.MatchString(cleaned) {
		return true
	}
	lower := strings.ToLower(cleaned)
	if strings.HasPrefix(lower, "project:") || strings.HasPrefix(lower, "module:") {
		return true
	}
	if isCamelCaseIdentifier(cleaned) || isAcronymWithVersion(cleaned) {
		return true
	}
	return false
}

func isSubsectionHeading(s string) (bool, string, string) {
	cleaned := cleanMarkdownTokens(s)
	lower := strings.ToLower(cleaned)

	type subMatch struct {
		keys []string
		kind string
	}
	matches := []subMatch{
		{[]string{"issue identified", "issues identified", "issue:", "issues:"}, "issue"},
		{[]string{"activities performed", "activity performed", "activities:", "activity:"}, "activities"},
		{[]string{"solution implemented", "solutions implemented", "solution:", "fix:", "fixes:"}, "solution"},
		{[]string{"outcome", "outcomes", "output:", "result:", "results:"}, "outcome"},
	}

	for _, m := range matches {
		for _, k := range m.keys {
			if strings.HasPrefix(lower, k) {
				rest := strings.TrimSpace(cleaned[len(k):])
				rest = strings.TrimPrefix(rest, ":")
				rest = strings.TrimSpace(rest)
				return true, m.kind, rest
			}
		}
	}
	return false, "", ""
}

func checkProjectHeading(line string, lines []string, idx int) (bool, string) {
	// Exclude non-projects
	if isHorizontalRule(line) || isTOCHeader(line) || isDocumentTitle(line) || isConclusionHeading(line) {
		return false, ""
	}
	if isSub, _, _ := isSubsectionHeading(line); isSub {
		return false, ""
	}
	if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "•") {
		return false, ""
	}

	// If the immediate next line is a subsection heading (Activities Performed, Issue Identified, etc.),
	// then this line is a task title, NOT a project heading!
	if idx+1 < len(lines) {
		if isSub, _, _ := isSubsectionHeading(lines[idx+1]); isSub {
			return false, ""
		}
	}

	t := strings.TrimSpace(line)

	// Markdown Level 1 Header: # ProjectName or # Part A – Project
	if strings.HasPrefix(t, "# ") {
		pName := cleanProjectName(t)
		if IsValidProjectName(pName) {
			return true, pName
		}
	}

	cleaned := cleanMarkdownTokens(t)
	if !IsValidProjectName(cleaned) {
		return false, ""
	}

	// Part A - Project
	rePart := regexp.MustCompile(`^Part\s+[A-Za-z]\s*[–\-—:]\s*`)
	if rePart.MatchString(cleaned) {
		pName := cleanProjectName(cleaned)
		if IsValidProjectName(pName) {
			return true, pName
		}
	}

	// Project: or Module:
	lower := strings.ToLower(cleaned)
	if strings.HasPrefix(lower, "project:") || strings.HasPrefix(lower, "module:") {
		pName := cleanProjectName(cleaned)
		if IsValidProjectName(pName) {
			return true, pName
		}
	}

	// Single CamelCase Identifier (e.g. EMACloudContentManager)
	if isCamelCaseIdentifier(cleaned) {
		return true, cleaned
	}

	// Acronym with version (e.g. ERA 3.0, MKCL ERA, CDP API)
	if isAcronymWithVersion(cleaned) {
		return true, cleaned
	}

	// Plain text lookahead heuristic
	wordCount := len(strings.Fields(cleaned))
	hasPeriod := strings.Contains(cleaned, ".") && !containsVersionNumber(cleaned)

	if !hasPeriod && wordCount <= 4 && idx+2 < len(lines) {
		nextClean := cleanMarkdownTokens(lines[idx+1])
		nextWordCount := len(strings.Fields(nextClean))
		nextHasPeriod := strings.Contains(nextClean, ".") && !containsVersionNumber(nextClean)

		afterClean := cleanMarkdownTokens(lines[idx+2])
		afterWordCount := len(strings.Fields(afterClean))
		isAfterSub, _, _ := isSubsectionHeading(lines[idx+2])

		// Line is short (<= 4 words), next is short task title (<= 8 words), line after is long description or subsection
		if nextWordCount <= 8 && !nextHasPeriod && (afterWordCount > 8 || strings.Contains(afterClean, ".") || isAfterSub) {
			pName := cleanProjectName(cleaned)
			if IsValidProjectName(pName) {
				return true, pName
			}
		}
	}

	return false, ""
}

func checkTaskTitle(line string, lines []string, idx int, currentProject string) (bool, string) {
	if isHorizontalRule(line) || isTOCHeader(line) || isDocumentTitle(line) || isConclusionHeading(line) {
		return false, ""
	}
	if isSub, _, _ := isSubsectionHeading(line); isSub {
		return false, ""
	}
	if strings.HasPrefix(line, "-") || strings.HasPrefix(line, "*") || strings.HasPrefix(line, "•") {
		return false, ""
	}

	t := strings.TrimSpace(line)

	// Markdown Level 2 Header: ## 1. Task Title or ## Task Title
	if strings.HasPrefix(t, "## ") {
		title := cleanDisplayTitle(t)
		if title != "" {
			return true, title
		}
	}

	cleaned := cleanMarkdownTokens(t)
	wordCount := len(strings.Fields(cleaned))
	hasPeriod := strings.Contains(cleaned, ".") && !containsVersionNumber(cleaned)

	// Numbered list item like "1. PowerShell Fallback" or "1) PowerShell Fallback"
	reNum := regexp.MustCompile(`^\d+[\.\)]\s+([A-Za-z].+)`)
	if match := reNum.FindStringSubmatch(cleaned); len(match) > 1 {
		matchedTitle := match[1]
		if len(strings.Fields(matchedTitle)) <= 12 && !strings.HasSuffix(matchedTitle, ".") {
			return true, cleanDisplayTitle(matchedTitle)
		}
	}

	// If next line is a subsection heading (e.g. "Activities Performed" or "Issue Identified"),
	// this line is definitely a task title!
	if idx+1 < len(lines) {
		if isSub, _, _ := isSubsectionHeading(lines[idx+1]); isSub {
			if wordCount <= 15 && !hasPeriod {
				return true, cleanDisplayTitle(cleaned)
			}
		}
	}

	// If currentProject is set, short title-like line without period
	if currentProject != "" && wordCount <= 8 && !hasPeriod && !containsVersionNumber(cleaned) && !strings.HasSuffix(cleaned, ":") {
		// Verify next line is not a project heading
		if idx+1 < len(lines) {
			next := lines[idx+1]
			if !isProjectHeadingFast(next) {
				return true, cleanDisplayTitle(cleaned)
			}
		} else {
			return true, cleanDisplayTitle(cleaned)
		}
	}

	return false, ""
}

func cleanProjectName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	s = strings.TrimPrefix(s, "#")
	s = strings.TrimPrefix(s, "#")
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "Project:")
	s = strings.TrimPrefix(s, "project:")
	s = strings.TrimPrefix(s, "Module:")
	s = strings.TrimPrefix(s, "module:")
	s = cleanMarkdownTokens(s)

	// Strip "Part A – " or "Part B - "
	rePart := regexp.MustCompile(`^Part\s+[A-Za-z]\s*[–\-—:]\s*`)
	if rePart.MatchString(s) {
		s = rePart.ReplaceAllString(s, "")
	}
	return strings.TrimSpace(s)
}

func cleanDisplayTitle(s string) string {
	s = strings.TrimSpace(s)
	// Strip markdown links [Title](#anchor) -> Title
	reLink := regexp.MustCompile(`\[(.*?)\]\(.*?\)`)
	s = reLink.ReplaceAllString(s, "$1")
	// Strip markdown headers
	s = strings.TrimPrefix(s, "###")
	s = strings.TrimPrefix(s, "##")
	s = strings.TrimPrefix(s, "#")
	s = strings.TrimSpace(s)
	// Strip leading numbers like "1. ", "2. ", "10) "
	reNum := regexp.MustCompile(`^\d+[\.\)]\s*`)
	s = reNum.ReplaceAllString(s, "")
	// Strip bullets
	s = strings.TrimPrefix(s, "-")
	s = strings.TrimPrefix(s, "•")
	s = strings.TrimPrefix(s, "*")
	s = strings.TrimSpace(s)
	// Strip bold / italics markdown
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "`", "")
	return strings.TrimSpace(s)
}

func cleanMarkdownTokens(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "####")
	s = strings.TrimPrefix(s, "###")
	s = strings.TrimPrefix(s, "##")
	s = strings.TrimPrefix(s, "#")
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "`", "")
	return strings.TrimSpace(s)
}

func stripMarkdownFormatting(s string) string {
	s = strings.TrimSpace(s)
	// Replace markdown link [Text](url) with Text
	reLink := regexp.MustCompile(`\[(.*?)\]\(.*?\)`)
	s = reLink.ReplaceAllString(s, "$1")
	// Remove bold / italics / code blocks
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "`", "")
	return s
}

func containsVersionNumber(s string) bool {
	matched, _ := regexp.MatchString(`\b\d+\.\d+\b`, s)
	return matched
}

func isAcronymWithVersion(s string) bool {
	parts := strings.Fields(s)
	if len(parts) == 0 || len(parts) > 3 {
		return false
	}
	first := parts[0]
	allUpper := true
	for _, r := range first {
		if unicode.IsLetter(r) && !unicode.IsUpper(r) {
			allUpper = false
			break
		}
	}
	if allUpper && len(first) >= 2 {
		return true
	}
	return false
}

func isCamelCaseIdentifier(s string) bool {
	if strings.Contains(s, " ") {
		return false
	}
	camelRe := regexp.MustCompile(`^[A-Z]+[a-z0-9]+[A-Za-z0-9]*$`)
	return camelRe.MatchString(s)
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(text, "\n")
}
