package dashboard

import (
	json "encoding/json/v2"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"9router/proxy/internal/handlerutil"
	"9router/proxy/internal/providers"
)

// autoOptimalComboKind marks the master combo built by HandleAutoOptimalCombo.
// Like the auto-family combos it is machine-generated but stays fully editable:
// it is a starting point the user can rename, prune or delete.
const autoOptimalComboKind = "auto-optimal"

// AutoOptimalComboID is the stable id of the master optimal combo, so
// regenerating upserts the same row instead of stacking duplicates.
const AutoOptimalComboID = "auto-optimal"

// autoOptimalComboName is the name the master combo answers to.
const autoOptimalComboName = "Auto: Optimal"

// optimalMaxModels bounds the master fallback chain. A wide catalog can expose
// hundreds of chat models; a fallback chain only needs enough depth to survive
// a few provider outages, and an unbounded row would make the combos page and
// every request that walks it unnecessarily heavy. When the cut applies the
// response reports truncated:true so the limit is visible, never silent.
const optimalMaxModels = 50

// Quality tiers used to order a fallback chain. Frontier first: once the free
// tier has failed, the next most useful answer is the strongest paid model, not
// the cheapest one. Free-ness is a separate, higher-priority dimension.
const (
	qualityFast     = 0
	qualityBalanced = 1
	qualityFrontier = 2
)

// optimalFastTokens mark a small/cheap model. Checked before frontier tokens so
// "o3-mini" and "gpt-5.4-mini" count as fast rather than frontier.
var optimalFastTokens = map[string]struct{}{
	"mini": {}, "nano": {}, "flash": {}, "haiku": {}, "turbo": {},
	"lite": {}, "small": {}, "tiny": {}, "micro": {}, "instant": {},
}

// optimalFrontierTokens mark a top-of-the-line model.
var optimalFrontierTokens = map[string]struct{}{
	"opus": {}, "ultra": {}, "pro": {}, "o1": {}, "o3": {}, "o4": {},
	"reasoner": {}, "thinking": {}, "max": {}, "r1": {},
}

// noPriority sorts a provider with no priority row last.
const noPriority = 1 << 30

// optimalModel is one candidate model with its resolved ordering key.
type optimalModel struct {
	key      string // "alias/model"
	free     bool
	quality  int
	priority int
}

// HandleAutoOptimalCombo handles POST /api/combos/auto-optimal.
//
// One click builds an optimal combo set from the usable chat models of the
// providers the user actually has a connection for: one combo per model family
// (the same rows Auto Group by Model writes, ordered optimally) plus a single
// master "Auto: Optimal" fallback chain spanning every family. Re-running the
// button upserts both, so it refreshes the setup rather than stacking rows.
func (h *DashboardHandler) HandleAutoOptimalCombo(w http.ResponseWriter, r *http.Request) {
	groups, err := h.usableModelFamilies(r.Context())
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	priorities, err := h.connectionPriorities()
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	families, err := h.upsertFamilyCombos(groups, priorities)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	ordered := optimalOrder(groups, priorities)
	truncated := false
	if len(ordered) > optimalMaxModels {
		ordered = ordered[:optimalMaxModels]
		truncated = true
	}
	if len(ordered) == 0 {
		handlerutil.WriteJSONError(w, http.StatusConflict,
			"no usable chat models found among providers with connections")
		return
	}

	modelsJSON, err := json.Marshal(ordered)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, "failed to encode models")
		return
	}
	if err := h.upsertCombo(AutoOptimalComboID, autoOptimalComboName, autoOptimalComboKind, string(modelsJSON)); err != nil {
		handlerutil.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"id":        AutoOptimalComboID,
		"models":    ordered,
		"count":     len(ordered),
		"truncated": truncated,
		"families":  families,
	})
}

// upsertCombo creates the combo row or rewrites it in place, so regenerating an
// auto combo refreshes its models instead of adding a second row.
func (h *DashboardHandler) upsertCombo(id, name, kind, modelsJSON string) error {
	existing, err := h.Repo.GetComboById(id)
	if err != nil {
		return fmt.Errorf("upsertCombo: load %s: %w", id, err)
	}
	if existing == nil {
		return h.Repo.CreateCombo(id, name, kind, modelsJSON, "fallback")
	}
	return h.Repo.UpdateCombo(id, name, kind, modelsJSON, "fallback")
}

// optimalOrder flattens every family's members into one optimally ordered chain.
func optimalOrder(groups map[string][]string, priorities map[string]int) []string {
	var all []string
	for _, members := range groups {
		all = append(all, members...)
	}
	return orderModels(all, priorities)
}

// orderModels sorts "alias/model" entries into fallback order: free tier first,
// then strongest quality, then the user's preferred provider (lower priority
// number wins), then the id itself for a stable, diffable result.
func orderModels(members []string, priorities map[string]int) []string {
	scored := make([]optimalModel, 0, len(members))
	for _, key := range members {
		alias, model := splitAliasModel(key)
		scored = append(scored, optimalModel{
			key:      key,
			free:     providers.IsFreeTierModel(model),
			quality:  qualityRank(model),
			priority: priorityFor(alias, priorities),
		})
	}
	sort.SliceStable(scored, func(i, j int) bool { return lessOptimal(scored[i], scored[j]) })

	out := make([]string, len(scored))
	for i, s := range scored {
		out[i] = s.key
	}
	return out
}

// lessOptimal is the ordering comparator: free before paid, frontier before
// balanced before fast, lower provider priority first, id last for stability.
func lessOptimal(a, b optimalModel) bool {
	if a.free != b.free {
		return a.free
	}
	if a.quality != b.quality {
		return a.quality > b.quality
	}
	if a.priority != b.priority {
		return a.priority < b.priority
	}
	return a.key < b.key
}

// qualityRank classifies a model id into a rough quality tier from the role
// words in its name. It is a heuristic for fallback ordering only — never a
// routing decision, so an unknown id lands in the balanced tier rather than
// being dropped.
func qualityRank(modelID string) int {
	tokens := strings.FieldsFunc(strings.ToLower(modelID), func(r rune) bool {
		switch r {
		case '-', '/', '_', '.', ':', ' ':
			return true
		}
		return false
	})
	for _, t := range tokens {
		if _, ok := optimalFastTokens[t]; ok {
			return qualityFast
		}
	}
	for _, t := range tokens {
		if _, ok := optimalFrontierTokens[t]; ok {
			return qualityFrontier
		}
	}
	return qualityBalanced
}

// priorityFor resolves a provider alias to its best (lowest) connection
// priority, or noPriority when the provider has none.
func priorityFor(alias string, priorities map[string]int) int {
	if p, ok := priorities[alias]; ok {
		return p
	}
	return noPriority
}

// connectionPriorities maps each provider (id and alias) with an active
// connection to its lowest connection priority, so a combo prefers the account
// the user ranked first. Providers with no explicit priority sort last.
func (h *DashboardHandler) connectionPriorities() (map[string]int, error) {
	conns, err := h.Repo.GetProviderConnections("", true)
	if err != nil {
		return nil, fmt.Errorf("connectionPriorities: %w", err)
	}
	out := make(map[string]int, len(conns))
	for _, c := range conns {
		p := noPriority
		if c.Priority != nil {
			p = *c.Priority
		}
		for _, key := range []string{c.Provider, providers.GetProviderAlias(c.Provider)} {
			if key == "" {
				continue
			}
			if cur, ok := out[key]; !ok || p < cur {
				out[key] = p
			}
		}
	}
	return out, nil
}
