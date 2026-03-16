package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Styling ---
var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#FAFAFA")).Background(lipgloss.Color("#056139")).Padding(0, 1).MarginBottom(1)
	subTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#056139")).Background(lipgloss.Color("#FAFAFA")).Padding(0, 1).MarginTop(1).MarginBottom(1)
	headerStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#00D7FF")).Bold(true)
	moneyStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	pctStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500"))
	docStyle      = lipgloss.NewStyle().Margin(1, 2)
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#777777")).MarginTop(2)
	taxStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(true)
)

// --- App States ---
type appState int

const (
	stateInput appState = iota
	stateLoading
	stateResult
)

// --- TUI Model ---
type model struct {
	state      appState
	step       int
	inputs     []textinput.Model
	spin       spinner.Model
	resultView string
}

// Custom message for our artificial loading delay
type calculationDoneMsg struct{}

func initialModel() model {
	// Initialize the spinner animation
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := model{state: stateInput, step: 0, spin: s}
	m.inputs = make([]textinput.Model, 9) // Increased to 9 inputs

	// Step Prompts with Explicit Defaults
	steps := []string{
		"1. Gross Annual Income (BDT) [Default: 0 / Skip]",
		"2. Custom Basic Salary (BDT) [Default: 0 for Auto 60%]",
		"3. Total Annual Expense (BDT) [Default: 0 / Skip]",
		"4. Location (dhaka/other) [Default: other]",
		"5. Family Size [Default: 3]",
		"6. Do you have kids? (y/n) [Default: n]",
		"7. Do you own your home? (y/n) [Default: n]",
		"8. Home-support staff? (y/n) [Default: n]",
		"9. Mode (balanced/conservative/comfortable) [Default: balanced]",
	}

	for i := range m.inputs {
		t := textinput.New()
		t.Placeholder = steps[i]
		if i == 0 {
			t.Focus()
		}
		m.inputs[i] = t
	}
	return m
}

func (m model) Init() tea.Cmd {
	return textinput.Blink
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			return m, tea.Quit
		case "r":
			if m.state == stateResult {
				return initialModel(), textinput.Blink
			}
		case "enter":
			if m.state == stateInput {
				if m.step < len(m.inputs)-1 {
					m.step++
					for i := range m.inputs {
						m.inputs[i].Blur()
					}
					m.inputs[m.step].Focus()
					return m, nil
				}
				// Reached the end of inputs, trigger loading state
				m.state = stateLoading
				return m, tea.Batch(m.spin.Tick, triggerCalculation())
			}
		}

	case spinner.TickMsg:
		if m.state == stateLoading {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}

	case calculationDoneMsg:
		m.state = stateResult
		m.resultView = generateResults(m)
		return m, nil
	}

	if m.state == stateInput {
		var cmd tea.Cmd
		m.inputs[m.step], cmd = m.inputs[m.step].Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m model) View() string {
	if m.state == stateLoading {
		return docStyle.Render(fmt.Sprintf("\n\n   %s Analyzing Tax Laws and Computing Liabilities...\n\n", m.spin.View()))
	}

	if m.state == stateResult {
		return docStyle.Render(m.resultView + helpStyle.Render("\nPress 'r' to restart • Press 'q' to quit"))
	}

	// Input State View
	s := titleStyle.Render(" TAX COMPANION (BANGLADESH) ") + "\n\n"
	for i := 0; i <= m.step; i++ {
		s += fmt.Sprintf("%s\n%s\n\n", headerStyle.Render(m.inputs[i].Placeholder), m.inputs[i].View())
	}
	return docStyle.Render(s)
}

// Simulates processing time for the animation
func triggerCalculation() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(1200 * time.Millisecond) // 1.2 second animation delay
		return calculationDoneMsg{}
	}
}

// --- Logic & Generators ---

func generateResults(m model) string {
	var sb strings.Builder

	// Parse inputs with defaults
	grossIncome, _ := strconv.ParseFloat(getVal(m.inputs[0].Value(), "0"), 64)
	customBasic, _ := strconv.ParseFloat(getVal(m.inputs[1].Value(), "0"), 64)
	totalExpense, _ := strconv.ParseFloat(getVal(m.inputs[2].Value(), "0"), 64)
	loc := strings.ToLower(getVal(m.inputs[3].Value(), "other"))
	fam, _ := strconv.Atoi(getVal(m.inputs[4].Value(), "3"))
	kids := strings.ToLower(getVal(m.inputs[5].Value(), "n")) == "y"
	own := strings.ToLower(getVal(m.inputs[6].Value(), "n")) == "y"
	staff := strings.ToLower(getVal(m.inputs[7].Value(), "n")) == "y"
	mode := strings.ToLower(getVal(m.inputs[8].Value(), "balanced"))

	// 1. Salary & Tax Exemptions
	if grossIncome > 0 {
		sb.WriteString(subTitleStyle.Render(" SALARY BREAKDOWN & TAXABLE INCOME ") + "\n")

		// Determine Basic (Use custom if provided, otherwise 60% of gross)
		var basic int
		if customBasic > 0 {
			basic = int(customBasic)
		} else {
			basic = int(math.Round(grossIncome * 0.60))
		}

		hra := int(math.Round(grossIncome * 0.30))
		medical := int(math.Round(grossIncome * 0.05))
		conveyance := int(grossIncome) - basic - hra - medical

		// Standard BD Exemptions
		exemptHRA := math.Min(float64(basic)*0.50, 300000)
		exemptMed := math.Min(float64(basic)*0.10, 120000)
		exemptConv := math.Min(float64(conveyance), 30000)
		totalExempt := exemptHRA + exemptMed + exemptConv
		taxableIncome := grossIncome - totalExempt

		salKeys := []string{"Basic Pay", "House Rent Allowance", "Medical Allowance", "Conveyance"}
		salVals := []int{basic, hra, medical, conveyance}

		for i, k := range salKeys {
			sb.WriteString(fmt.Sprintf("%-30s | %s\n", k, moneyStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(salVals[i])))))
		}

		sb.WriteString(strings.Repeat("-", 45) + "\n")
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "TOTAL GROSS SALARY", moneyStyle.Bold(true).Render(fmt.Sprintf("Tk %10s", formatMoney(int(grossIncome))))))
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "TOTAL EXEMPTIONS", pctStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(totalExempt))))))
		sb.WriteString(strings.Repeat("-", 45) + "\n")
		sb.WriteString(fmt.Sprintf("%-30s | %s\n\n", "NET TAXABLE INCOME", taxStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(taxableIncome))))))

		// 2. ACTUAL TAX LIABILITY CALCULATION
		sb.WriteString(subTitleStyle.Render(" TAX LIABILITY (SLAB BREAKDOWN) ") + "\n")
		taxSb := calculateTax(taxableIncome)
		sb.WriteString(taxSb)
		sb.WriteString("\n")
	}

	// 3. Expense Estimation
	if totalExpense > 0 {
		sb.WriteString(subTitleStyle.Render(" IT-10BB FORM ESTIMATE RESULTS ") + "\n")
		pcts, amts := computeAllocation(totalExpense, loc, fam, kids, own, staff, mode)

		expKeys := []string{
			"Food, Clothing and Essentials", "Accommodation Expense", "Electricity",
			"Gas, Water, Sewer and Garbage", "Phone, Internet, TV & Subs",
			"Home-Support & Other Expenses", "Education Expenses", "Festival, Party, Events",
		}

		for _, k := range expKeys {
			if pcts[k] == 0 && amts[k] == 0 {
				continue
			}
			pStr := pctStyle.Render(fmt.Sprintf("%3d%%", pcts[k]))
			aStr := moneyStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(amts[k])))
			sb.WriteString(fmt.Sprintf("%-30s | %s | %s\n", k, pStr, aStr))
		}

		sb.WriteString(strings.Repeat("-", 55) + "\n")
		totalStr := moneyStyle.Bold(true).Render(fmt.Sprintf("Tk %10s", formatMoney(int(totalExpense))))
		sb.WriteString(fmt.Sprintf("%-30s | %s | %s\n", "TOTAL ANNUAL EXPENDITURE", pctStyle.Render("100%"), totalStr))
	} else if grossIncome == 0 {
		sb.WriteString("No income or expense provided. Nothing to calculate!\n")
	}

	return sb.String()
}

// --- Bangladesh Tax Slab Logic ---
func calculateTax(taxable float64) string {
	if taxable <= 350000 {
		return "🎉 No tax liability! (Income is at or below the Tk 3,50,000 threshold)\n"
	}

	// BD Standard Individual Slabs
	slabs := []struct {
		limit float64
		rate  float64
		label string
	}{
		{350000, 0.00, "First 3.5 Lakh (0%)"},
		{100000, 0.05, "Next 1 Lakh (5%)"},
		{400000, 0.10, "Next 4 Lakh (10%)"},
		{500000, 0.15, "Next 5 Lakh (15%)"},
		{500000, 0.20, "Next 5 Lakh (20%)"},
		{math.MaxFloat64, 0.25, "Remaining (25%)"},
	}

	var sb strings.Builder
	var totalTax float64
	remaining := taxable

	sb.WriteString(fmt.Sprintf("%-22s | %-16s | %s\n", "SLAB", "AMOUNT TAXED", "TAX GENERATED"))
	sb.WriteString(strings.Repeat("-", 62) + "\n")

	for _, slab := range slabs {
		if remaining <= 0 {
			break
		}
		amountInSlab := math.Min(remaining, slab.limit)
		taxForSlab := amountInSlab * slab.rate
		totalTax += taxForSlab
		remaining -= amountInSlab

		amtStr := pctStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(amountInSlab))))
		taxStr := moneyStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(taxForSlab))))

		// Highlight non-zero tax liabilities in red
		if taxForSlab > 0 {
			taxStr = taxStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(taxForSlab))))
		}

		sb.WriteString(fmt.Sprintf("%-22s | %16s | %16s\n", slab.label, amtStr, taxStr))
	}

	sb.WriteString(strings.Repeat("-", 62) + "\n")

	// Minimum Tax Check (Usually Tk 5000 for Dhaka/Chattogram, Tk 4000 other cities, Tk 3000 outside)
	// Kept generic minimum checking simple here as an example.
	if totalTax > 0 && totalTax < 3000 {
		totalTax = 3000
		sb.WriteString("* Applied minimum tax threshold (Tk 3,000)\n")
	}

	sb.WriteString(fmt.Sprintf("%-22s | %-16s | %s\n", "TOTAL TAX PAYABLE", "", taxStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(totalTax))))))

	return sb.String()
}

// Helper to handle defaults
func getVal(input, defaultVal string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return defaultVal
	}
	return trimmed
}

// --- Math & Allocation Logistics ---
func computeAllocation(total float64, loc string, familySize int, hasKids, ownHome, staff bool, mode string) (map[string]int, map[string]int) {
	weights := map[string]float64{
		"Food, Clothing and Essentials": 30.0, "Accommodation Expense": 28.0,
		"Electricity": 2.5, "Gas, Water, Sewer and Garbage": 3.0,
		"Phone, Internet, TV & Subs": 3.5, "Home-Support & Other Expenses": 7.0,
		"Education Expenses": 10.0, "Festival, Party, Events": 6.0,
	}

	if !hasKids {
		weights["Education Expenses"] = 0.0
	}

	if loc == "dhaka" || loc == "city" || loc == "metro" {
		weights["Accommodation Expense"] *= 1.20
		weights["Food, Clothing and Essentials"] *= 1.05
		weights["Home-Support & Other Expenses"] *= 1.10
	} else {
		weights["Accommodation Expense"] *= 0.90
	}

	extraPeople := math.Max(0, float64(familySize-2))
	if extraPeople > 0 {
		weights["Food, Clothing and Essentials"] *= (1 + 0.05*extraPeople)
		if hasKids {
			weights["Education Expenses"] *= (1 + 0.04*extraPeople)
		}
	}

	if ownHome {
		weights["Accommodation Expense"] *= 0.60
		weights["Home-Support & Other Expenses"] *= 1.05
	}
	if !staff {
		weights["Home-Support & Other Expenses"] *= 0.4
	}

	switch mode {
	case "conservative":
		weights["Festival, Party, Events"] *= 0.6
		weights["Home-Support & Other Expenses"] *= 0.7
		weights["Food, Clothing and Essentials"] *= 1.08
	case "comfortable":
		weights["Festival, Party, Events"] *= 1.3
		weights["Home-Support & Other Expenses"] *= 1.2
		weights["Food, Clothing and Essentials"] *= 1.05
	}

	var totalWeight float64
	for _, w := range weights {
		totalWeight += w
	}

	intPcts := make(map[string]int)
	remainders := make([]struct {
		val float64
		key string
	}, 0)
	currentTotalPct := 0

	for k, w := range weights {
		raw := (w / totalWeight) * 100.0
		val := int(raw)
		intPcts[k] = val
		currentTotalPct += val
		remainders = append(remainders, struct {
			val float64
			key string
		}{raw - float64(val), k})
	}

	sort.Slice(remainders, func(i, j int) bool { return remainders[i].val > remainders[j].val })
	for i := 0; i < (100 - currentTotalPct); i++ {
		intPcts[remainders[i].key]++
	}

	intAmts := make(map[string]int)
	totalAllocated := 0
	amtRemainders := make([]struct {
		val float64
		key string
	}, 0)

	for k, p := range intPcts {
		rawAmt := total * float64(p) / 100.0
		amt := int(rawAmt)
		intAmts[k] = amt
		totalAllocated += amt
		amtRemainders = append(amtRemainders, struct {
			val float64
			key string
		}{rawAmt - float64(amt), k})
	}

	drift := int(math.Round(total - float64(totalAllocated)))
	if drift > 0 {
		sort.Slice(amtRemainders, func(i, j int) bool { return amtRemainders[i].val > amtRemainders[j].val })
		for i := 0; i < drift; i++ {
			intAmts[amtRemainders[i].key]++
		}
	}

	return intPcts, intAmts
}

func formatMoney(n int) string {
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		return s
	}
	var res []string
	for i := len(s); i > 0; i -= 3 {
		start := i - 3
		if start < 0 {
			start = 0
		}
		res = append([]string{s[start:i]}, res...)
	}
	return strings.Join(res, ",")
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v", err)
		os.Exit(1)
	}
}
