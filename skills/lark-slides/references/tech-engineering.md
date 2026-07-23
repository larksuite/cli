# Tech & Engineering

Scope: sharing and presenting technical solutions, system architecture reviews, AI / data platform proposals, security design reviews, production incident retrospectives, technology selection, testing and validation, API / SDK documentation, and product technical training, among others.

Readers may be fellow engineers, but they may also be product partners or customers. The goal is for readers to understand how the system works, judge whether the solution is good, and see the trade-offs, risks, and next steps clearly.

## 1. Core Character

**Medium-to-high information density, with emphasis on technical explanation and on processes and mechanisms.** Pages are built around architectures, flows, sequences, data, comparisons, and real evidence; text explains the why, while visuals reveal the relationships.

- **Professional**: no empty talk of "high availability" or "high performance." Write out the metrics, environment, boundaries, dependencies, trade-offs, failure conditions, and recovery paths.
- **Concise**: a relationship one diagram can explain must not meander into an essay; nor may necessary assumptions, evidence, and limitations be omitted just to keep the layout clean.
- **Concrete**: architecture diagrams must show call directions, data flow, control flow, and trust or failure boundaries; code, configuration, and logs appear only when they genuinely support the argument.
- **Evidence first**: every page advances one judgment. The title gives the conclusion, the body gives the evidence, and it explains what the evidence means for the solution, the risks, or the next steps.
- **Restrained**: the wow factor comes from making a complex problem clear — not from glow, particles, walls of cards, and hollow "tech vibes."

## 2. General Prohibitions

The following are red lines running through the entire deck; they take effect before all layout and visual rules, and unless the user explicitly requests otherwise, none may be violated.

- **No cards by default**: unless the user explicitly requests it, strictly forbid using rounded rectangles or rectangular cards to build hierarchy or alignment. Line segments, whitespace, and font-size/weight differences are better solutions.
- **No evenly divided compositions**: unless no other layout is available, do not default to one-third splits, four-way splits, or 2×2 matrices — including formulaic patterns such as "title + three parallel blocks + conclusion."
- **No mediocre, common, or AI-typical color schemes**: unless the user explicitly requests them, strictly forbid blue-and-white pairings, blue-purple gradients, cyan-purple neon, rainbow flares, glassmorphism cards, and glowing borders.
- **No elements that clash with the overall style**: no styles from outside the chosen style may appear, such as using rounded icons or rounded rectangles within a sharp style.

## 3. How to Present Technical Content

### 3.1 Explain how the system works first

When first explaining a system or module, cover these first:

- inputs, outputs, and success criteria;
- the scope of the system and its external dependencies;
- the normal path, exception paths, and fallback paths;
- where data comes from, what processing it goes through, and where it ends up;
- which positions are the critical boundaries for performance, security, cost, or reliability.

### 3.2 Put design rationale and costs side by side

- Explain why A was chosen and B was not; the comparison dimensions must come from real goals.
- Write out the costs corresponding to the benefits, including performance, cost, complexity, team capability, vendor lock-in, observability, and migration risk.
- Give the conditions under which it works and the conditions under which it fails; do not present a local optimum as the universal optimum.
- Distinguish current state, target state, candidate options, hypothetical inference, and verified results.
- Launch plans specify validation gates, the gradual rollout method, rollback conditions, and recovery paths.

### 3.3 Evidence must be verifiable

- Metrics come with their source, time, version, test environment, load, sample, and measurement basis.
- Show P50 / P95 / P99, variability, outliers, or confidence intervals as the question demands — not just averages.
- Team inferences, illustrative data, and yet-to-be-verified assumptions must be explicitly labeled.
- When evidence is missing, use "to be filled in," "assumption," or a placeholder; never fabricate benchmarks, logs, cases, or failure causes.
- Explain terms and abbreviations at their first appearance, and keep naming consistent afterward.

## 4. Narrative Skeletons by Engineering Task

One primary scenario determines the whole deck's structure; other scenarios only supply necessary additions — never stack another visual language on top.

| Engineering task | What the reader must judge | Recommended narrative order |
|---|---|---|
| System architecture review | Whether boundaries, dependencies, bottlenecks, and the evolution path are sound | Goals and constraints → current state → problems → alternatives → target architecture → migration and validation |
| AI / data platform proposal | Whether the data, models, and service chain are trustworthy; how quality, latency, and cost are traded off | User tasks → data and model pipeline → core mechanisms → evaluation evidence → risk guardrails → launch loop |
| Security design review | Whether trust boundaries, attack paths, controls, and residual risk are clear | Assets and boundaries → threats → attack paths → controls → residual risk → monitoring and response |
| Production incident retrospective | What the impact was, how the failure happened, and whether the fixes can prevent recurrence | Impact summary → timeline → direct cause → systemic factors → fixes → prevention validation |
| Technology selection review | Which option better fits the goals, and which assumptions the conclusion is sensitive to | Goals and criteria → candidates → like-for-like comparison → trade-offs → recommendation → exit conditions |
| Testing / validation strategy | Whether risks are covered, and whether test layers and release gates suffice | Risk model → layered validation → environment and data → gates → coverage gaps → release decision |
| External tech talk | Why the core mechanism is valuable, whether the evidence is credible, and where the boundaries lie | Problem → mechanism → technical implementation → evidence → limitations → adoption path |

When no better structure fits, start from "decision summary → goals and constraints → current state / problems → mechanism / architecture → key evidence → alternatives and trade-offs → implementation / validation → risks and next steps," then trim and adapt to the scenario.

## 5. Page Rhythm and Information Density

- Default to medium-high density, but keep only one main judgment and one main evidence object per page.
- Alternate text-explanation pages and visual-evidence pages; avoid consecutive stacks of structurally identical bullet lists.
- For complex concepts, give the whole first, then expand layer by layer along stable coordinates; do not redraw the same architecture on every page.
- Show the normal path, exception paths, and migration path in layers; keep the unchanged parts and highlight only what changes.
- Accent pages prioritize key architectures, key comparisons, incident causality, or the final decision — do not manufacture climaxes with big-type slogans.
- Section transitions appear only when a genuine change of rhythm is needed; short materials are not force-fitted with tables of contents and section covers.
- End by returning to the decision, the validation results, or the next steps — never replace the conclusion with a lone "Thank you."

### Common page types

- **Concept / background page:** the necessary explanation paired with one main figure, whitespace spread around the main object.
- **Architecture / flow page:** graphics dominate; text keeps only the conclusion, the legend, and necessary side notes.
- **Comparison / selection page:** options compared on the same scale, dimensions, and coordinates.
- **Metrics / benchmark page:** charts lead; test conditions, baselines, and conclusions stay close to the chart.
- **Incident / timeline page:** chronological order is the main axis; impact, evidence, and handling stay close to their corresponding events.
- **Implementation / migration page:** phases, dependencies, gates, and rollback points are laid out according to their real relationships.
- **Appendix / sources page:** may be denser, but must stay scannable via columns, numbering, and stable line spacing.

## 6. Architecture Diagrams and Flowcharts

### 6.1 The relationship determines the graphic

| Relationship | Preferred graphic | Information that must be conveyed |
|---|---|---|
| System regions and boundaries | Nested or side-by-side right-angled regions | System scope, external dependencies, trust / failure boundaries |
| Calls and data flow | Node chains or networks with directional arrows | The two parties of each call, protocol / data, sync or async |
| Multi-role interaction | Sequence diagrams | Roles, message directions, waits, and exception branches |
| Deployment and failure domains | Topology diagrams | Regions / clusters / instances, redundancy, and isolation scope |
| State transitions | State machines | States, trigger conditions, transitions, and error states |
| Option comparison | Side-by-side topologies or matrices at the same scale | Identical evaluation dimensions, structural differences, and costs |
| Incident causality | Timeline + causal chain / fault tree | Events, evidence, direct causes, contributing factors, and control gaps |
| Validation coverage | Layered matrices or mapping diagrams | Risks, test layers, pass criteria, and coverage gaps |

### 6.2 Diagramming discipline

- Arrows must have direction and meaning; label the protocol, data, frequency, capacity, or trigger conditions where necessary.
- Distinguish data flow, control flow, and exception paths; redundant encoding with color, line style, and labels is allowed.
- External boundaries, internal modules, and critical paths use different but stable grammars.
- Connectors terminate at node edges and never cross through text; when there are too many crossings, rearrange, layer, or split the page.
- Bold the critical paths or use the accent color; other paths recede to neutral; no glow and no 3D arrows.
- For complex architectures, overview first, then zoom into details; keep naming, colors, and coordinate cues consistent.
- Legends explain only the encodings that cannot be labeled in place; anything that can sit beside its node or path is labeled directly.

## 7. Data Charts, Tables, Code, and Screenshots

### Data charts

- Trends use lines, comparisons use bars or side-by-side dot plots, composition uses stacks, latency distributions use histograms / box plots / quantile plots, and capacity-cost relationships may use scatter plots or sensitivity matrices.
- Remove default frames, 3D effects, gradients, heavy gridlines, and meaningless legends.
- Main series use the structural color or critical-path color; baselines and secondary series use grayscale.
- Key values, inflection points, and anomalies are labeled directly, with the "why" and the "what it means."
- Test environment, version, load, sample, units, and baseline appear alongside the chart.

### Tables

- Headers, grouping, status, numbers, and notes form clear hierarchy; default themes are not kept.
- Notes left-aligned, numbers aligned by decimal point or unit, and status columns fixed in position.
- Use thin separators and light group backgrounds; no thick outer frames, rainbow headers, or large red/green fills.
- Options are compared on the same dimensions; missing and non-comparable items are explicitly marked.

### Code, configuration, and screenshots

- Keep only the minimal excerpt that supports the argument, and first tell the reader what to look at.
- Code, interfaces, paths, and fields use monospaced fonts; syntax highlighting only accentuates the key lines or changes.
- For configuration comparison, prefer a minimal diff or a key-fields table; logs are timestamped, identify the source component, and have sensitive data redacted or anonymized.
- Crop screenshots tightly to the evidence boundary, keeping them right-angled rectangles; do not uniformly add device frames, rounded corners, or shadows.
- Side-by-side screenshots keep equal height and width, identical zoom, and the same baseline.

## 8. Visual System

### 8.1 Layout

- A title conveys the conclusion in one sentence, and its visual weight must not exceed the main evidence.
- Use stable safe margins and a title axis; ordinary pages prefer a single column, a left-right split, or text above and figure below.
- Architecture nodes land on a regular grid; pages of the same kind keep the same skeleton.
- Body content is organized by whitespace, alignment, and thin rules by default; cards are used only when content needs delimiting.
- Panels are used only for system scope, code, configuration, or key conclusions — mostly right-angled or minimally rounded, with thin strokes and light fills, and no generic shadows.
- Reading-type materials may keep the document name, section, page number, version, and confidentiality level; live-presented materials keep only the necessary identity information.

### 8.2 Colors and fonts

- Reviews, printing, and long-form reading usually use light backgrounds; dark-room launches may use a restrained dark background.
- When the brand has existing visual assets, inherit them first; without brand constraints, build the palette along the lines of "background color, structural color, critical-path color, status colors, neutrals."
- Each color carries a stable meaning throughout the deck; critical states are additionally distinguished by text, line style, or shape.
- Titles and body text prefer highly legible sans-serif faces; when a document feel is needed, titles may use a restrained serif.
- Code uses monospaced fonts; one deck usually uses no more than two body-type families.
- When content grows, first condense the text, split pages, or adjust the text-to-graphic ratio — only then shrink font size slightly.

### 8.3 Lines, icons, and effects

- Ordinary borders and auxiliary lines are thin and even; only critical paths get visibly bolder.
- Within one diagram, arrows, corners, endpoints, dash rhythms, and corner radii stay consistent.
- Technical structures prefer right angles or minimal rounding; circles and curves appear only when semantics require them.
- Architecture, flows, data, screenshots, and real product visuals take precedence over decorative illustration.
- Icons must come from one stroke system and serve identification; do not mix emoji, 3D, realistic, and system-default icons.
- Body surfaces stay clean: no noise, particles, grid-light effects, glassmorphism, or meaningless gradients.

## 9. Sample References

Samples are only for extracting mechanisms; wholesale replication is forbidden. Organize the Style first according to the current readers, evidence, brand, and usage; precise color values, fonts, covers, section pages, and footers are not inherited by default.

### Sample A: Warm Orange & Cool Blue — White-Base Engineering Document Style

- **Traits:** white background with black body text; the warm color carries titles and critical paths, the cool color carries nodes and structure; fixed header and page numbers; suited to reading and printing.
- **Borrowable:** the warm/cool semantic division, a stable document axis, reuse of architecture coordinates, and hierarchical distinction through boundaries / nodes / paths.
- **Good for reference:** architecture reviews, technical white papers, platform proposals, and materials that need to unfold the same architecture step by step.
- **Not inherited automatically:** the orange and blue color values, Inter, the gradient-arc cover, orange table headers, and the fixed header position.

### Sample B: Four-Color Flat — Engineering Walkthrough Style

- **Traits:** white background with dark-gray text, a few high-saturation semantic colors, flat structural diagrams, and solid-color section pages; suited to live walkthroughs and cross-role communication.
- **Borrowable:** semantic color coding, solid-color section pauses, text/graphic zoning, direct labeling, and equal-width comparison columns.
- **Good for reference:** product technical training, role collaboration, and talks balancing concepts and flows.
- **Not inherited automatically:** the fixed four colors, solid-per-chapter backgrounds, the dual-ended footer, and specific brand fonts.

### Sample C: White Base & Deep Ink Blue — Two-Color Short-Line Engineering Walkthrough

- **Traits:** deep ink-blue titles, short colored lines, and a white base forming a stable walkthrough skeleton; explanation pages alternate with evidence pages; visuals float directly on the white ground.
- **Borrowable:** the fixed title axis, weak navigation, alternation of explanation and evidence, and reduced container noise.
- **Good for reference:** live tech talks, building concepts step by step, and materials centered on screenshots and mechanism diagrams.
- **Not inherited automatically:** the orange/green section colors, diagonal section color blocks, the deep-purple ending, the fixed short-line sizes, and the font combination.

Do not write "choose Sample A / B / C" in the plan. If the result still clearly looks like a direct replica of some Sample once the text is hidden, keep adjusting — but do not break relationship expression and consistency just to be novel.

## 10. Pre-Delivery Checklist

- Are inputs, outputs, dependencies, boundaries, the normal path, exception paths, and fallback paths clear?
- Do key connectors have direction and meaning, and are current state, target state, assumptions, and verified results distinguished?
- Do metrics carry environment, version, sample, load, units, baseline, and time window?
- Does each page carry only one main judgment, and does the evidence truly support the title?
- Does the visual weight fall on the evidence, and are colors, shapes, line styles, and naming consistent?
- Are there unsourced data, default Office charts, walls of cards, decorative "tech vibes," or Sample replicas?
- Was technical evidence altered, weakened, or falsified for the sake of visual unity?
