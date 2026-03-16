package main

import (
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- Styling ---
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#056139")). // BD Flag Green
			Padding(0, 1).
			MarginBottom(1)

	headerStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#00D7FF")).Bold(true)
	moneyStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00"))
	pctStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFA500"))
	docStyle    = lipgloss.NewStyle().Margin(1, 2)
)

// --- Logic & Calculations ---

// computeSalaryBreakdown generates a standard BD tax-optimized salary structure
func computeSalaryBreakdown(gross float64) map[string]int {
	basic := int(math.Round(gross * 0.60))
	hra := int(math.Round(gross * 0.30))
	medical := int(math.Round(gross * 0.05))

	// Ensure the sum matches exactly by dumping any rounding differences into conveyance
	conveyance := int(gross) - basic - hra - medical

	return map[string]int{
		"Basic Pay (60%)":             basic,
		"House Rent Allowance (30%)":  hra,
		"Medical Allowance (5%)":      medical,
		"Conveyance / Transport (5%)": conveyance,
	}
}

func computeAllocation(total float64, loc string, familySize int, hasKids, ownHome, staff bool, mode string) (map[string]int, map[string]int) {
	weights := map[string]float64{
		"Food, Clothing and Essentials": 30.0,
		"Accommodation Expense":         28.0,
		"Electricity":                   2.5,
		"Gas, Water, Sewer and Garbage": 3.0,
		"Phone, Internet, TV & Subs":    3.5,
		"Home-Support & Other Expenses": 7.0,
		"Education Expenses (for kids)": 10.0,
		"Festival, Party, Events":       6.0,
	}

	if !hasKids {
		weights["Education Expenses (for kids)"] = 0.0
	}

	// Location Adjustments
	l := strings.ToLower(loc)
	if l == "dhaka" || l == "city" || l == "metro" {
		weights["Accommodation Expense"] *= 1.20
		weights["Food, Clothing and Essentials"] *= 1.05
		weights["Home-Support & Other Expenses"] *= 1.10
	} else {
		weights["Accommodation Expense"] *= 0.90
	}

	// Family Size
	extraPeople := math.Max(0, float64(familySize-2))
	if extraPeople > 0 {
		weights["Food, Clothing and Essentials"] *= (1 + 0.05*extraPeople)
		if hasKids {
			weights["Education Expenses (for kids)"] *= (1 + 0.04*extraPeople)
		}
	}

	if ownHome {
		weights["Accommodation Expense"] *= 0.60
		weights["Home-Support & Other Expenses"] *= 1.05
	}
	if !staff {
		weights["Home-Support & Other Expenses"] *= 0.4
	}

	// Modes
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

	// Normalize Percentages
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

	// Round-robin distribution for percentages
	sort.Slice(remainders, func(i, j int) bool { return remainders[i].val > remainders[j].val })
	for i := 0; i < (100 - currentTotalPct); i++ {
		intPcts[remainders[i].key]++
	}

	// Calculate BDT amounts
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

	// Fix BDT rounding drift
	drift := int(math.Round(total - float64(totalAllocated)))
	if drift > 0 {
		sort.Slice(amtRemainders, func(i, j int) bool { return amtRemainders[i].val > amtRemainders[j].val })
		for i := 0; i < drift; i++ {
			intAmts[amtRemainders[i].key]++
		}
	}

	return intPcts, intAmts
}

// --- TUI Model ---
type model struct {
	step   int
	inputs []textinput.Model
	done   bool
}

func initialModel() model {
	m := model{step: 0}
	m.inputs = make([]textinput.Model, 8)

	steps := []string{
		"Gross Annual Income (BDT) [Enter to skip breakdown]",
		"Total Annual Expense (BDT)",
		"Location (Dhaka/Other)",
		"Family Size",
		"Kids? (y/n)",
		"Own Home? (y/n)",
		"Staff? (y/n)",
		"Mode (balanced/conservative/comfortable)",
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

func (m model) Init() tea.Cmd { return textinput.Blink }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if m.step < len(m.inputs)-1 {
				m.step++
				for i := range m.inputs {
					m.inputs[i].Blur()
				}
				m.inputs[m.step].Focus()
				return m, nil
			}
			m.done = true
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.inputs[m.step], cmd = m.inputs[m.step].Update(msg)
	return m, cmd
}

func (m model) View() string {
	if m.done {
		return ""
	}

	s := titleStyle.Render(" TAX COMPANION (BANGLADESH) ") + "\n\n"
	for i := 0; i <= m.step; i++ {
		s += fmt.Sprintf("%s\n%s\n\n", headerStyle.Render(m.inputs[i].Placeholder), m.inputs[i].View())
	}
	return docStyle.Render(s)
}

func main() {
	p := tea.NewProgram(initialModel())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	m := finalModel.(model)

	// Parse inputs (shifted by 1 to accommodate Gross Income)
	grossIncome, _ := strconv.ParseFloat(m.inputs[0].Value(), 64)
	totalExpense, _ := strconv.ParseFloat(m.inputs[1].Value(), 64)
	loc := m.inputs[2].Value()
	fam, _ := strconv.Atoi(m.inputs[3].Value())
	kids := strings.ToLower(m.inputs[4].Value()) == "y"
	own := strings.ToLower(m.inputs[5].Value()) == "y"
	staff := strings.ToLower(m.inputs[6].Value()) == "y"
	mode := strings.ToLower(m.inputs[7].Value())
	if mode == "" {
		mode = "balanced"
	}

	// 1. Output Salary Breakdown if provided
	if grossIncome > 0 {
		fmt.Println(titleStyle.Render(" AUTO-GENERATED SALARY BREAKDOWN "))
		salBreakdown := computeSalaryBreakdown(grossIncome)

		salKeys := []string{"Basic Pay (60%)", "House Rent Allowance (30%)", "Medical Allowance (5%)", "Conveyance / Transport (5%)"}
		for _, k := range salKeys {
			label := fmt.Sprintf("%-35s", k)
			aStr := moneyStyle.Render(fmt.Sprintf("Tk %12s", formatMoney(salBreakdown[k])))
			fmt.Printf("%s | %s\n", label, aStr)
		}
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("%-35s | %s\n\n", "TOTAL GROSS SALARY", moneyStyle.Bold(true).Render(fmt.Sprintf("Tk %12s", formatMoney(int(grossIncome)))))
	}

	// 2. Output Expense Estimation if provided
	if totalExpense > 0 {
		pcts, amts := computeAllocation(totalExpense, loc, fam, kids, own, staff, mode)

		fmt.Println(titleStyle.Render(" IT-10BB FORM ESTIMATE RESULTS "))

		expKeys := []string{
			"Food, Clothing and Essentials", "Accommodation Expense", "Electricity",
			"Gas, Water, Sewer and Garbage", "Phone, Internet, TV & Subs",
			"Home-Support & Other Expenses", "Education Expenses (for kids)", "Festival, Party, Events",
		}

		for _, k := range expKeys {
			if pcts[k] == 0 && amts[k] == 0 {
				continue
			}
			label := fmt.Sprintf("%-35s", k)
			pStr := pctStyle.Render(fmt.Sprintf("%3d%%", pcts[k]))
			aStr := moneyStyle.Render(fmt.Sprintf("Tk %12s", formatMoney(amts[k])))
			fmt.Printf("%s | %s | %s\n", label, pStr, aStr)
		}

		fmt.Println(strings.Repeat("-", 60))
		totalStr := moneyStyle.Bold(true).Render(fmt.Sprintf("Tk %12s", formatMoney(int(totalExpense))))
		fmt.Printf("%-35s | %s | %s\n\n", "TOTAL ANNUAL EXPENDITURE", pctStyle.Render("100%"), totalStr)
	} else {
		fmt.Println("No total expense provided. Skipping IT-10BB estimation.")
	}
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
