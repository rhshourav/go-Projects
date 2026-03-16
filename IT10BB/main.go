package main

import (
	"fmt"
	"math"
	"os"
	"regexp"
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
	chartBarStyle = lipgloss.NewStyle().Background(lipgloss.Color("#00D7FF")).Foreground(lipgloss.Color("#000000")).Padding(0, 0)
	chartLabel    = lipgloss.NewStyle().Bold(true)
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

type calculationDoneMsg struct{}

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := model{state: stateInput, step: 0, spin: s}
	m.inputs = make([]textinput.Model, 19)

	steps := []string{
		"1. Gross Annual Income (BDT) [Default: 0 / Skip]",
		"2. Enter Custom Salary Breakdown? (y/n) [Default: n]",
		"   -> Basic Pay (annual BDT) [Default: 0]",
		"   -> House Rent Allowance (annual BDT) [Default: 0]",
		"   -> Medical Allowance (annual BDT) [Default: 0]",
		"   -> Food Allowance (annual BDT) [Default: 0]",
		"   -> Transport / Conveyance (annual BDT) [Default: 0]",
		"   -> Mobile & Other Allowance (annual BDT) [Default: 0]",
		"3. Total Annual Expense (BDT) [Default: 0 / Skip]",
		"4. Location (dhaka/other) [Default: other]",
		"5. Family Size [Default: 3]",
		"6. Do you have kids? (y/n) [Default: n]",
		"7. Do you own your home? (y/n) [Default: n]",
		"8. Home-support staff? (y/n) [Default: n]",
		"9. Mode (balanced/conservative/comfortable) [Default: balanced]",
		"10. Previous Year's Gross Income (BDT) [Default: 0 / Optional]",
		"11. Net Wealth (BDT) [Default: 0]",
		"12. Apply Net Wealth Surcharge? (y/n) [Default: n]",
		"13. Surcharge Percent (number like 10 OR 'auto') [Default: auto]",
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
				// Branching logic for custom breakdown
				if m.step == 1 {
					wantsCustom := strings.ToLower(getVal(m.inputs[1].Value(), "n")) == "y"
					if !wantsCustom {
						// skip breakdown indices 2..7 -> jump to index 8
						m.step = 8
					} else {
						m.step++
					}
				} else if m.step < len(m.inputs)-1 {
					m.step++
				} else {
					// End of inputs -> trigger calculation
					m.state = stateLoading
					return m, tea.Batch(m.spin.Tick, triggerCalculation())
				}

				for i := range m.inputs {
					m.inputs[i].Blur()
				}
				m.inputs[m.step].Focus()
				return m, nil
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
		return docStyle.Render(fmt.Sprintf("\n\n   %s Computing tax, surcharge and allocations...\n\n", m.spin.View()))
	}

	if m.state == stateResult {
		return docStyle.Render(m.resultView + helpStyle.Render("\nPress 'r' to restart • Press 'q' to quit"))
	}

	s := titleStyle.Render(" TAX COMPANION (BANGLADESH) ") + "\n\n"
	for i := 0; i <= m.step; i++ {
		// hide breakdown if user chose 'n'
		wantsCustom := strings.ToLower(getVal(m.inputs[1].Value(), "n")) == "y"
		if i >= 2 && i <= 7 && !wantsCustom {
			continue
		}
		s += fmt.Sprintf("%s\n%s\n\n", headerStyle.Render(m.inputs[i].Placeholder), m.inputs[i].View())
	}
	return docStyle.Render(s)
}

func triggerCalculation() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(900 * time.Millisecond)
		return calculationDoneMsg{}
	}
}

// ------------------ numeric parsing & evaluator ------------------

// parseNumeric accepts free-form numeric inputs:
// - arithmetic: "12809 * 23", "34440+89", "(1+2)*3"
// - shorthand units: "1k", "1K" => 1,000; "1 lakh"|"1lakh"|"1 lac" => 100,000; "1cr"|"1 crore" => 10,000,000
// - commas are allowed: "1,234,567"
// Returns computed float64 (0 if blank or parse error).
func parseNumeric(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// normalize
	s = strings.ToLower(strings.ReplaceAll(s, ",", ""))
	// Replace unit patterns like "1 lakh" or "1lakh" or "1k"
	unitPatterns := map[*regexp.Regexp]string{
		regexp.MustCompile(`(?i)(\d+(\.\d+)?)\s*(k|thousand)\b`):      "*1000",
		regexp.MustCompile(`(?i)(\d+(\.\d+)?)\s*(m|mn|million)\b`):    "*1000000",
		regexp.MustCompile(`(?i)(\d+(\.\d+)?)\s*(lakh|lac|lacs)\b`):   "*100000",
		regexp.MustCompile(`(?i)(\d+(\.\d+)?)\s*(cr|crore|crores)\b`): "*10000000",
		regexp.MustCompile(`(?i)(\d+(\.\d+)?)\s*(tk|taka)\b`):         "*1",
	}
	for re, repl := range unitPatterns {
		s = re.ReplaceAllString(s, "$1"+repl)
	}
	// Now evaluate arithmetic expression
	val, err := evalExpression(s)
	if err != nil {
		// fallback: try to parse as plain float
		if f, e2 := strconv.ParseFloat(s, 64); e2 == nil {
			return f
		}
		return 0
	}
	return val
}

// Eval arithmetic expression with + - * / and parentheses using shunting-yard and RPN evaluation.
func evalExpression(expr string) (float64, error) {
	// Tokenize
	type token struct {
		typ string // "num", "op", "(", ")"
		val string
	}
	tokens := []token{}
	i := 0
	expr = strings.TrimSpace(expr)
	var prevTok *token
	for i < len(expr) {
		ch := expr[i]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\n':
			i++
		case (ch >= '0' && ch <= '9') || ch == '.':
			j := i
			for j < len(expr) && ((expr[j] >= '0' && expr[j] <= '9') || expr[j] == '.') {
				j++
			}
			num := expr[i:j]
			tokens = append(tokens, token{typ: "num", val: num})
			prevTok = &tokens[len(tokens)-1]
			i = j
		case ch == '+' || ch == '-' || ch == '*' || ch == '/':
			// handle unary minus: if '-' and (start or prev is op or prev is '('), treat as unary by inserting a 0 before it.
			if ch == '-' && (prevTok == nil || prevTok.typ == "op" || prevTok.val == "(") {
				// insert 0 num
				tokens = append(tokens, token{typ: "num", val: "0"})
				prevTok = &tokens[len(tokens)-1]
			}
			tokens = append(tokens, token{typ: "op", val: string(ch)})
			prevTok = &tokens[len(tokens)-1]
			i++
		case ch == '(' || ch == ')':
			tokens = append(tokens, token{typ: string(ch), val: string(ch)})
			prevTok = &tokens[len(tokens)-1]
			i++
		default:
			// unexpected char (could be part of leftover unit text) -> return error
			return 0, fmt.Errorf("invalid character in expression: %c", ch)
		}
	}

	// Shunting-yard: convert to RPN
	outQueue := []token{}
	opStack := []token{}
	prec := map[string]int{"+": 1, "-": 1, "*": 2, "/": 2}
	for _, tk := range tokens {
		if tk.typ == "num" {
			outQueue = append(outQueue, tk)
		} else if tk.typ == "op" {
			for len(opStack) > 0 {
				top := opStack[len(opStack)-1]
				if top.typ == "op" && ((prec[top.val] > prec[tk.val]) || (prec[top.val] == prec[tk.val])) {
					// pop
					outQueue = append(outQueue, top)
					opStack = opStack[:len(opStack)-1]
					continue
				}
				break
			}
			opStack = append(opStack, tk)
		} else if tk.typ == "(" {
			opStack = append(opStack, tk)
		} else if tk.typ == ")" {
			found := false
			for len(opStack) > 0 {
				top := opStack[len(opStack)-1]
				opStack = opStack[:len(opStack)-1]
				if top.typ == "(" {
					found = true
					break
				}
				outQueue = append(outQueue, top)
			}
			if !found {
				return 0, fmt.Errorf("mismatched parentheses")
			}
		}
	}
	for len(opStack) > 0 {
		top := opStack[len(opStack)-1]
		opStack = opStack[:len(opStack)-1]
		if top.typ == "(" || top.typ == ")" {
			return 0, fmt.Errorf("mismatched parentheses")
		}
		outQueue = append(outQueue, top)
	}

	// Evaluate RPN
	stack := []float64{}
	for _, tk := range outQueue {
		if tk.typ == "num" {
			f, err := strconv.ParseFloat(tk.val, 64)
			if err != nil {
				return 0, err
			}
			stack = append(stack, f)
		} else if tk.typ == "op" {
			if len(stack) < 2 {
				return 0, fmt.Errorf("invalid expression")
			}
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			var res float64
			switch tk.val {
			case "+":
				res = a + b
			case "-":
				res = a - b
			case "*":
				res = a * b
			case "/":
				if b == 0 {
					return 0, fmt.Errorf("division by zero")
				}
				res = a / b
			default:
				return 0, fmt.Errorf("unknown operator %s", tk.val)
			}
			stack = append(stack, res)
		}
	}
	if len(stack) != 1 {
		return 0, fmt.Errorf("invalid expression after eval")
	}
	return stack[0], nil
}

// ------------------ tax & salary logic ------------------

// calculateTax returns slab breakdown string, total tax, and slab contributions map.
func calculateTax(taxable float64) (string, float64, map[string]float64) {
	out := make(map[string]float64)

	if taxable <= 350000 {
		out["total_tax"] = 0
		return "🎉 No tax liability! (Income is at or below the Tk 3,50,000 threshold)\n", 0, out
	}

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
		if taxForSlab > 0 {
			taxStr = taxStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(taxForSlab))))
		}

		sb.WriteString(fmt.Sprintf("%-22s | %16s | %16s\n", slab.label, amtStr, taxStr))
		out[slab.label] = taxForSlab
	}

	sb.WriteString(strings.Repeat("-", 62) + "\n")

	if totalTax > 0 && totalTax < 3000 {
		totalTax = 3000
		sb.WriteString("* Applied minimum tax threshold (Tk 3,000)\n")
	}

	sb.WriteString(fmt.Sprintf("%-22s | %-16s | %s\n", "TOTAL TAX PAYABLE", "", taxStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(totalTax))))))

	out["total_tax"] = totalTax
	return sb.String(), totalTax, out
}

// deriveSalaryBreakdownFromGross: deterministic mapping when only a total annual salary is provided.
// Returns: basicAnnual, hra, medical, conveyance, food, festivalTotal, others
// The implementation ensures integer taka math and exact sum to input gross.
func deriveSalaryBreakdownFromGross(gross float64) (basicAnnual, hra, medical, conveyance, food, festivalTotal, others int) {
	if gross <= 0 {
		return 0, 0, 0, 0, 0, 0, 0
	}
	// Use algebraic assumption:
	// festival total = 2 months' basic, and basic is around half of salary after festival.
	// Derived proportion: basic ≈ (6/13) * gross ; festival ≈ basic/6 ≈ (1/13) * gross
	basicFloat := gross * 6.0 / 13.0
	festivalFloat := basicFloat / 6.0
	remainingAfterFestival := gross - festivalFloat
	allowancesPool := remainingAfterFestival - basicFloat

	components := []struct {
		key string
		val float64
	}{
		{"basic", basicFloat},
		{"festival", festivalFloat},
		{"allowances_pool", allowancesPool},
	}

	ints := make(map[string]int)
	remainders := make([]struct {
		key string
		rem float64
	}, 0)
	totalInt := 0
	for _, c := range components {
		v := c.val
		i := int(v)
		ints[c.key] = i
		totalInt += i
		remainders = append(remainders, struct {
			key string
			rem float64
		}{c.key, v - float64(i)})
	}

	grossInt := int(math.Round(gross))
	diff := grossInt - totalInt
	sort.Slice(remainders, func(i, j int) bool { return remainders[i].rem > remainders[j].rem })
	for i := 0; i < diff; i++ {
		ints[remainders[i%len(remainders)].key]++
	}

	allowPool := ints["allowances_pool"]
	// split allowances_pool into hra, medical, conveyance, food, others
	hraI := int(math.Round(float64(allowPool) * 0.30))
	medI := int(math.Round(float64(allowPool) * 0.05))
	convI := int(math.Round(float64(allowPool) * 0.03))
	foodI := int(math.Round(float64(allowPool) * 0.03))
	othersI := allowPool - (hraI + medI + convI + foodI)
	if othersI < 0 {
		short := -othersI
		othersI = 0
		// absorb into HRA if needed
		hraI -= short
		if hraI < 0 {
			hraI = 0
		}
	}

	basicAnnual = ints["basic"]
	festivalTotal = ints["festival"]
	hra = hraI
	medical = medI
	conveyance = convI
	food = foodI
	others = othersI

	// final guard to ensure exact sum
	sum := basicAnnual + festivalTotal + hra + medical + conveyance + food + others
	if sum != grossInt {
		others += grossInt - sum
	}

	return basicAnnual, hra, medical, conveyance, food, festivalTotal, others
}

func generateResults(m model) string {
	var sb strings.Builder

	// Parse numeric fields with parseNumeric (accepts expressions and units)
	grossIncome := parseNumeric(getVal(m.inputs[0].Value(), "0"))
	wantsCustom := strings.ToLower(getVal(m.inputs[1].Value(), "n")) == "y"

	// custom breakdown parse (use parseNumeric for each)
	cBasic := parseNumeric(getVal(m.inputs[2].Value(), "0"))
	cHra := parseNumeric(getVal(m.inputs[3].Value(), "0"))
	cMed := parseNumeric(getVal(m.inputs[4].Value(), "0"))
	cFood := parseNumeric(getVal(m.inputs[5].Value(), "0"))
	cTrans := parseNumeric(getVal(m.inputs[6].Value(), "0"))
	cMob := parseNumeric(getVal(m.inputs[7].Value(), "0"))

	// rest
	totalExpense := parseNumeric(getVal(m.inputs[8].Value(), "0"))
	loc := strings.ToLower(getVal(m.inputs[9].Value(), "other"))
	fam, _ := strconv.Atoi(getVal(m.inputs[10].Value(), "3"))
	kids := strings.ToLower(getVal(m.inputs[11].Value(), "n")) == "y"
	own := strings.ToLower(getVal(m.inputs[12].Value(), "n")) == "y"
	staff := strings.ToLower(getVal(m.inputs[13].Value(), "n")) == "y"
	mode := strings.ToLower(getVal(m.inputs[14].Value(), "balanced"))

	// new inputs
	prevIncome := parseNumeric(getVal(m.inputs[15].Value(), "0"))
	netWealth := parseNumeric(getVal(m.inputs[16].Value(), "0"))
	applySurcharge := strings.ToLower(getVal(m.inputs[17].Value(), "n")) == "y"
	surchargeInput := strings.ToLower(getVal(m.inputs[18].Value(), "auto"))

	// SALARY BREAKDOWN
	if grossIncome > 0 || wantsCustom {
		sb.WriteString(subTitleStyle.Render(" SALARY BREAKDOWN & TAXABLE INCOME ") + "\n")

		var basicAnnual, hraA, medicalA, conveyanceA, foodA, festivalA, othersA int

		if wantsCustom {
			// If user provided custom annual components, use those (round to int)
			basicAnnual = int(math.Round(cBasic))
			hraA = int(math.Round(cHra))
			medicalA = int(math.Round(cMed))
			foodA = int(math.Round(cFood))
			conveyanceA = int(math.Round(cTrans))
			othersA = int(math.Round(cMob))
			// festival: derive from basicAnnual as 2 months basic
			festivalA = int(math.Round(float64(basicAnnual) / 6.0))
			// If custom components don't sum to grossIncome, adjust 'others' to match gross exactly
			sum := basicAnnual + hraA + medicalA + foodA + conveyanceA + festivalA + othersA
			if sum != int(math.Round(grossIncome)) {
				othersA += int(math.Round(grossIncome)) - sum
			}
		} else {
			// Auto-derive from gross with deterministic algebra (see function doc)
			basicAnnual, hraA, medicalA, conveyanceA, foodA, festivalA, othersA = deriveSalaryBreakdownFromGross(grossIncome)
		}

		// Compute exemption and taxable income
		totalExempt := math.Min(grossIncome/3.0, 450000)
		taxableIncome := grossIncome - totalExempt

		// Show breakdown (ensure integer formatting)
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "Basic (annual)", moneyStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(basicAnnual)))))
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "House Rent Allowance (annual)", moneyStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(hraA)))))
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "Medical Allowance (annual)", moneyStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(medicalA)))))
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "Food Allowance (annual)", moneyStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(foodA)))))
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "Conveyance (annual)", moneyStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(conveyanceA)))))
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "Festival Bonus (total)", moneyStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(festivalA)))))
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "Other Allowances (annual)", moneyStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(othersA)))))

		sb.WriteString(strings.Repeat("-", 60) + "\n")
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "TOTAL GROSS SALARY", moneyStyle.Bold(true).Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(grossIncome)))))))
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "TOTAL EXEMPTION (Act 2023)", pctStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(totalExempt)))))))
		sb.WriteString(strings.Repeat("-", 60) + "\n")
		sb.WriteString(fmt.Sprintf("%-30s | %s\n\n", "NET TAXABLE INCOME", taxStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(taxableIncome)))))))

		// TAX CALC
		sb.WriteString(subTitleStyle.Render(" TAX LIABILITY (CURRENT YEAR - SLAB BREAKDOWN) ") + "\n")
		taxSb, totalTaxCur, slabMap := calculateTax(taxableIncome)
		sb.WriteString(taxSb + "\n")

		// PREVIOUS YEAR TAX (optional)
		var prevTaxSb string
		var totalTaxPrev float64
		if prevIncome > 0 {
			sb.WriteString(subTitleStyle.Render(" PREVIOUS YEAR TAX (OPTIONAL) ") + "\n")
			prevExempt := math.Min(prevIncome/3.0, 450000)
			prevTaxable := prevIncome - prevExempt
			prevTaxSb, totalTaxPrev, _ = calculateTax(prevTaxable)
			sb.WriteString(fmt.Sprintf("%-30s | %s\n", "PREVIOUS GROSS INCOME", moneyStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(prevIncome)))))))
			sb.WriteString(fmt.Sprintf("%-30s | %s\n", "PREVIOUS EXEMPTION", pctStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(prevExempt)))))))
			sb.WriteString(fmt.Sprintf("%-30s | %s\n\n", "PREVIOUS TAXABLE INCOME", taxStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(prevIncome-prevExempt)))))))
			sb.WriteString(prevTaxSb + "\n")
		}

		combinedBeforeSurcharge := totalTaxCur + totalTaxPrev
		sb.WriteString(strings.Repeat("=", 60) + "\n")
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "TOTAL TAX (CURRENT)", taxStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(totalTaxCur)))))))
		if prevIncome > 0 {
			sb.WriteString(fmt.Sprintf("%-30s | %s\n", "TOTAL TAX (PREVIOUS)", taxStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(totalTaxPrev)))))))
		}
		sb.WriteString(fmt.Sprintf("%-30s | %s\n\n", "COMBINED (BEFORE SURCHARGE)", taxStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(combinedBeforeSurcharge)))))))

		// NET WEALTH SURCHARGE
		var surchargeRate float64
		var surchargeAmount float64
		threshold := 40000000.0 // 4 crore
		if applySurcharge && netWealth > threshold {
			if surchargeInput == "auto" || surchargeInput == "" {
				switch {
				case netWealth <= 50000000:
					surchargeRate = 0.10
				case netWealth <= 100000000:
					surchargeRate = 0.20
				default:
					surchargeRate = 0.35
				}
			} else {
				// parse custom percent
				pStr := strings.TrimSuffix(surchargeInput, "%")
				p, err := strconv.ParseFloat(pStr, 64)
				if err == nil && p >= 0 {
					surchargeRate = p / 100.0
				} else {
					surchargeRate = 0.10 // fallback
				}
			}
			surchargeAmount = combinedBeforeSurcharge * surchargeRate
			sb.WriteString(subTitleStyle.Render(" NET WEALTH SURCHARGE CHECK ") + "\n")
			sb.WriteString(fmt.Sprintf("%-30s | %s\n", "NET WEALTH (user)", moneyStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(netWealth)))))))
			sb.WriteString(fmt.Sprintf("%-30s | %s\n", "THRESHOLD", pctStyle.Render("Tk 40,000,000")))
			sb.WriteString(fmt.Sprintf("%-30s | %s\n", "SURCHARGE %", pctStyle.Render(fmt.Sprintf("%.1f%%", surchargeRate*100))))
			sb.WriteString(fmt.Sprintf("%-30s | %s\n\n", "SURCHARGE AMOUNT", taxStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(surchargeAmount)))))))
		} else {
			if applySurcharge {
				sb.WriteString(subTitleStyle.Render(" NET WEALTH SURCHARGE CHECK ") + "\n")
				sb.WriteString(fmt.Sprintf("%-30s | %s\n\n", "RESULT", pctStyle.Render("No surcharge applied (below threshold)")))
			}
		}

		finalTax := combinedBeforeSurcharge + surchargeAmount
		sb.WriteString(strings.Repeat("=", 60) + "\n")
		sb.WriteString(fmt.Sprintf("%-30s | %s\n", "FINAL TAX (AFTER SURCHARGE)", taxStyle.Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(finalTax)))))))
		sb.WriteString(strings.Repeat("=", 60) + "\n\n")

		// Charts
		sb.WriteString(subTitleStyle.Render(" VISUAL: TAX SUMMARY ") + "\n")
		sb.WriteString(renderTaxChart(combinedBeforeSurcharge, surchargeAmount, finalTax) + "\n\n")

		sb.WriteString(subTitleStyle.Render(" TAX SLAB CONTRIBUTIONS (visual) ") + "\n")
		sb.WriteString(renderSlabBars(slabMap) + "\n\n")
	}

	// EXPENSE ALLOCATION
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
		totalStr := moneyStyle.Bold(true).Render(fmt.Sprintf("Tk %10s", formatMoney(int(math.Round(totalExpense)))))
		sb.WriteString(fmt.Sprintf("%-30s | %s | %s\n", "TOTAL ANNUAL EXPENDITURE", pctStyle.Render("100%"), totalStr))

		sb.WriteString("\n" + subTitleStyle.Render(" VISUAL: EXPENSE ALLOCATION (top items) ") + "\n")
		sb.WriteString(renderExpenseChart(pcts, amts) + "\n")
	} else if grossIncome == 0 {
		sb.WriteString("No income or expense provided. Nothing to calculate!\n")
	}

	return sb.String()
}

// ------------------ charts ------------------

func renderTaxChart(tax float64, surcharge float64, final float64) string {
	maxVal := math.Max(tax, math.Max(surcharge, final))
	if maxVal <= 0 {
		return "No tax to display."
	}
	var lines []string
	lines = append(lines, renderBar("Tax", tax, maxVal))
	if surcharge > 0 {
		lines = append(lines, renderBar("Surcharge", surcharge, maxVal))
	}
	lines = append(lines, renderBar("Final Tax", final, maxVal))
	return strings.Join(lines, "\n")
}

func renderBar(label string, value, max float64) string {
	width := 36
	frac := 0.0
	if max > 0 {
		frac = value / max
	}
	filled := int(math.Round(frac * float64(width)))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	bar := strings.Repeat(" ", filled)
	empty := strings.Repeat(" ", width-filled)
	block := chartBarStyle.Render(bar) + lipgloss.NewStyle().Foreground(lipgloss.Color("#444444")).Render(empty)
	labelStr := fmt.Sprintf("%-12s %8s", label, fmt.Sprintf("Tk %s", formatMoney(int(math.Round(value)))))
	return chartLabel.Render(labelStr) + "  " + block
}

func renderSlabBars(slab map[string]float64) string {
	type kv struct {
		k string
		v float64
	}
	arr := make([]kv, 0)
	var max float64
	for k, v := range slab {
		if k == "total_tax" {
			continue
		}
		arr = append(arr, kv{k, v})
		if v > max {
			max = v
		}
	}
	if len(arr) == 0 {
		return "No slab contributions to display."
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].v > arr[j].v })
	if len(arr) > 5 {
		arr = arr[:5]
	}
	var sb strings.Builder
	for _, x := range arr {
		sb.WriteString(renderBar(x.k, x.v, max) + "\n")
	}
	return sb.String()
}

func renderExpenseChart(pcts map[string]int, amts map[string]int) string {
	type kv struct {
		k string
		p int
		a int
	}
	arr := make([]kv, 0)
	for k, p := range pcts {
		arr = append(arr, kv{k, p, amts[k]})
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].p > arr[j].p })
	if len(arr) == 0 {
		return "No expenses to chart."
	}
	if len(arr) > 5 {
		arr = arr[:5]
	}
	var max float64
	for _, x := range arr {
		if float64(x.p) > max {
			max = float64(x.p)
		}
	}
	var sb strings.Builder
	for _, x := range arr {
		sb.WriteString(renderBar(fmt.Sprintf("%s (%d%%)", x.k, x.p), float64(x.p), max) + "\n")
	}
	return sb.String()
}

// ------------------ allocation engine (exact totals) ------------------

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
		intPcts[remainders[i%len(remainders)].key]++
	}

	intAmts := make(map[string]int)
	totalAllocated := 0
	amtRemainders := make([]struct {
		val float64
		key string
	}, 0)

	// We'll work with integer taka:
	totalInt := int(math.Round(total))

	for k, p := range intPcts {
		rawAmt := float64(totalInt) * float64(p) / 100.0
		amt := int(rawAmt)
		intAmts[k] = amt
		totalAllocated += amt
		amtRemainders = append(amtRemainders, struct {
			val float64
			key string
		}{rawAmt - float64(amt), k})
	}

	drift := totalInt - totalAllocated
	if drift > 0 {
		sort.Slice(amtRemainders, func(i, j int) bool { return amtRemainders[i].val > amtRemainders[j].val })
		for i := 0; i < drift; i++ {
			intAmts[amtRemainders[i%len(amtRemainders)].key]++
		}
	} else if drift < 0 {
		// negative drift should be rare; adjust by removing from smallest remainder entries
		sort.Slice(amtRemainders, func(i, j int) bool { return amtRemainders[i].val < amtRemainders[j].val })
		for i := 0; i < -drift; i++ {
			intAmts[amtRemainders[i%len(amtRemainders)].key]--
		}
	}

	return intPcts, intAmts
}

// ------------------ helpers ------------------

func getVal(input, defaultVal string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return defaultVal
	}
	return trimmed
}

func formatMoney(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
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
	out := strings.Join(res, ",")
	if neg {
		return "-" + out
	}
	return out
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running program: %v\n", err)
		os.Exit(1)
	}
}
