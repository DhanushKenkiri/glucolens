package assistant

import (
	"fmt"
	"strings"

	"glucolens/internal/dataset"
)

const rules = `You are GlucoLens, the assistant in a patient app that helps people understand their checkup results. You answer with a dataset of %d anonymised checkup records. You cannot read the data yourself: call the tools, which run the analysis and return figures.

How to answer:
- Get every number about the dataset from a tool. Never guess or invent statistics. If the tools cannot answer, say so.
- Use the exact column names, values and outcome ids listed below in tool calls.
- Lead with the direct answer, then the key figures in **bold**, then one line on what it means. Keep it under about 180 words; use short bullet points when comparing groups. Round percentages to one decimal.
- Say "in this dataset", never "in general"; this dataset is not a sample of the general population.%s
- A link is not proof of cause. If age or another factor could explain a difference, say so; analyze_risk can stratify by a third column to check.
- For questions about the user, use their shared values (below) with get_percentile or find_similar_people. A percentile below 50 means lower than most people in the dataset, above 50 means higher than most; follow each value's "meaning" field exactly and never contradict it.
- You may explain medical terms from general knowledge; say when something is general information rather than from the dataset. Clinical ranges are general guides.
- Never diagnose, and do not give treatment or medication advice; suggest talking the results through with a clinician. If someone describes urgent symptoms such as chest pain, fainting or very high blood sugar with symptoms, tell them to get urgent medical care.
- If a question is unrelated to health or this dataset, briefly say what you can help with.`

// schemaSummary lists every analysable column compactly for the prompt.
func schemaSummary(ds *dataset.Dataset) string {
	var b strings.Builder
	b.WriteString("Columns (name: label, type and values):\n")
	for _, c := range ds.Columns {
		if !c.Analysable() {
			continue
		}
		fmt.Fprintf(&b, "- %s: %s", c.Name, c.Label)
		if c.Unit != "" {
			fmt.Fprintf(&b, " (%s)", c.Unit)
		}
		switch c.Kind {
		case dataset.KindCategorical:
			fmt.Fprintf(&b, "; one of %s", strings.Join(c.Levels, " | "))
		case dataset.KindBinary:
			fmt.Fprintf(&b, "; %s | %s", c.Levels[0], c.Levels[1])
		default:
			if c.Stats != nil {
				fmt.Fprintf(&b, "; number %s to %s", dataset.FormatNumber(c.Stats.Min, c.Decimals), dataset.FormatNumber(c.Stats.Max, c.Decimals))
			}
		}
		b.WriteByte('\n')
	}
	b.WriteString("\nOutcome presets for analyze_risk and rank_risk_factors (id: meaning):\n")
	for _, p := range ds.Outcomes {
		fmt.Fprintf(&b, "- %s: %s\n", p.ID, p.Label)
	}
	return b.String()
}

// patientSummary renders the user's shared values for the prompt.
func patientSummary(ds *dataset.Dataset, patient map[string]any) string {
	if len(patient) == 0 {
		return "The user has not shared their own checkup values. If they ask about themselves, suggest adding them under My values."
	}
	vals, _ := ds.NormalizeAll(patient)
	var lines []string
	for _, c := range ds.Columns {
		if v, ok := vals[c.Name]; ok {
			lines = append(lines, fmt.Sprintf("- %s: %s", c.Name, v.Display))
		}
	}
	return "The user's own checkup values (shared by them):\n" + strings.Join(lines, "\n")
}

// buildInstruction assembles the system instruction for one turn.
func buildInstruction(ds *dataset.Dataset, schema string, patient map[string]any) string {
	note := ""
	if c, ok := ds.Column("diagnosed_diabetes"); ok && c.Kind == dataset.KindBinary && ds.Rows > 0 {
		note = fmt.Sprintf(" About %.0f%% of people here have diagnosed diabetes, far more than in the general population.",
			float64(c.Counts[1])/float64(ds.Rows)*100)
	}
	return fmt.Sprintf(rules, ds.Rows, note) + "\n\n" + schema + "\n" + patientSummary(ds, patient)
}
