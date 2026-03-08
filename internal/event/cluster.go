package event

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const clusterWindow = 72 * time.Hour

var numberRegexp = regexp.MustCompile(`\d[\d,.]*`)

type processedArticle struct {
	ArticleInput
	normalizedTitle string
	eventType       string
	assets          []string
	topics          []string
	entityIDs       []string
	rumor           bool
	official        bool
	numbers         []string
	amountLike      bool
}

type cluster struct {
	members []processedArticle
}

func BuildRecords(inputs []ArticleInput, now time.Time) ([]Record, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}

	processed := make([]processedArticle, 0, len(inputs))
	for _, input := range inputs {
		if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.SourceID) == "" {
			continue
		}
		processed = append(processed, processArticle(input))
	}
	if len(processed) == 0 {
		return nil, nil
	}

	sort.Slice(processed, func(i, j int) bool {
		return processed[i].PublishedAt.Before(processed[j].PublishedAt)
	})

	clusters := make([]cluster, 0)
	for _, art := range processed {
		index, _ := findClusterIndex(art, clusters)
		if index == -1 {
			clusters = append(clusters, cluster{
				members: []processedArticle{art},
			})
			continue
		}
		clusters[index].members = append(clusters[index].members, art)
	}

	records := make([]Record, 0, len(clusters))
	for _, cl := range clusters {
		rec := buildRecord(cl.members, now)
		records = append(records, rec)
	}

	sort.Slice(records, func(i, j int) bool {
		if records[i].ImportanceScore == records[j].ImportanceScore {
			if records[i].ConfidenceScore == records[j].ConfidenceScore {
				return records[i].PublishedAt > records[j].PublishedAt
			}
			return records[i].ConfidenceScore > records[j].ConfidenceScore
		}
		return records[i].ImportanceScore > records[j].ImportanceScore
	})

	return records, nil
}

func findClusterIndex(art processedArticle, clusters []cluster) (int, float64) {
	bestIndex := -1
	bestScore := 0.0

	for i := range clusters {
		score, ok := clusterMatchScore(art, clusters[i])
		if !ok {
			continue
		}
		if score > bestScore {
			bestScore = score
			bestIndex = i
		}
	}

	return bestIndex, bestScore
}

func clusterMatchScore(candidate processedArticle, cl cluster) (float64, bool) {
	bestSimilarity := 0.0
	eligible := false

	for _, member := range cl.members {
		if absDuration(candidate.PublishedAt.Sub(member.PublishedAt)) > clusterWindow {
			continue
		}

		if violatesMergeGuards(candidate, member) {
			return 0, false
		}

		similarity := titleSimilarity(candidate.normalizedTitle, member.normalizedTitle)
		hasOverlap := intersects(candidate.assets, member.assets) || intersects(candidate.entityIDs, member.entityIDs)

		isEligible := (similarity >= MergeSimilarityThreshold && hasOverlap) || (similarity >= HighSimilarityEventTypeThreshold && candidate.eventType == member.eventType)
		if !isEligible {
			continue
		}

		eligible = true
		if similarity > bestSimilarity {
			bestSimilarity = similarity
		}
	}

	return bestSimilarity, eligible
}

func violatesMergeGuards(a, b processedArticle) bool {
	if len(a.assets) > 0 && len(b.assets) > 0 && !intersects(a.assets, b.assets) {
		return true
	}

	if a.amountLike && b.amountLike && len(a.numbers) > 0 && len(b.numbers) > 0 && a.numbers[0] != b.numbers[0] {
		return true
	}

	if (a.rumor && b.official) || (b.rumor && a.official) {
		return true
	}

	return false
}

func buildRecord(members []processedArticle, now time.Time) Record {
	canonical := chooseCanonicalArticle(members)
	assets := uniqueSorted(flattenAssets(members))
	topics := uniqueSorted(flattenTopics(members))
	entities := mergeEntities(assets, members)
	supporting := buildSupportingArticles(members)
	uniqueSources := uniqueSourceIDs(members)
	status := inferStatus(members, len(uniqueSources))
	hasOfficial := hasOfficialSource(members)
	conflict := hasUnresolvedConflict(members)

	publishedAt := earliestPublishedAt(members)
	firstSeenAt := earliestFirstSeenAt(members)

	sourceWeightMax := maxSourceWeight(members)
	sourceClusterScore := clamp(float64(len(uniqueSources))/5.0, 0, 1)
	recencyScore := recency(publishedAt, now)
	officialScore := 0.0
	if hasOfficial {
		officialScore = 1
	}
	entityImpactScore := entityImpact(assets, topics)

	importance := clamp(
		0.35*sourceWeightMax+
			0.20*sourceClusterScore+
			0.20*recencyScore+
			0.15*officialScore+
			0.10*entityImpactScore,
		0,
		1,
	)

	confidence := confidenceScore(len(uniqueSources), hasOfficial, conflict)
	marketRelevance := marketRelevanceScore(assets, topics, hasOfficial, inferEventTypeForMembers(members))
	rights := deriveRights(members)

	eventType := inferEventTypeForMembers(members)
	title := strings.TrimSpace(canonical.Title)
	publishedAtRFC3339 := publishedAt.UTC().Format(time.RFC3339)

	eventID := buildEventID(publishedAt, title, members)

	summary1L := title
	if len(uniqueSources) > 1 {
		summary1L = fmt.Sprintf("%s (%d supporting sources)", title, len(uniqueSources))
	}

	summary5L := []string{
		fmt.Sprintf("Event type: %s.", eventType),
		fmt.Sprintf("Sources: %d.", len(uniqueSources)),
		fmt.Sprintf("Confidence: %.2f.", confidence),
	}
	if len(assets) > 0 {
		summary5L = append(summary5L, fmt.Sprintf("Assets: %s.", strings.Join(assets, ", ")))
	}
	if len(summary5L) > 5 {
		summary5L = summary5L[:5]
	}

	return Record{
		EventID:              eventID,
		Category:             "crypto",
		Status:               status,
		EventType:            eventType,
		Title:                title,
		Summary1L:            summary1L,
		Summary5L:            summary5L,
		Assets:               assets,
		Topics:               topics,
		Entities:             entities,
		PublishedAt:          publishedAtRFC3339,
		UpdatedAt:            now.UTC().Format(time.RFC3339),
		FirstSeenAt:          firstSeenAt.UTC().Format(time.RFC3339),
		LastVerifiedAt:       now.UTC().Format(time.RFC3339),
		ImportanceScore:      importance,
		MarketRelevanceScore: marketRelevance,
		ConfidenceScore:      confidence,
		SourceClusterSize:    len(members),
		SupportingArticles:   supporting,
		Rights:               rights,
		MarkdownURL:          fmt.Sprintf("output/events/%s.md", eventID),
	}
}

func processArticle(input ArticleInput) processedArticle {
	normalized := normalizeForSimilarity(input.Title)
	assets := extractAssets(input.Title)
	topics := inferTopics(input.Title)
	entityIDs := extractEntityIDs(input.Title)
	eventType := inferEventType(input.Title)
	numbers := extractNumberTokens(input.Title)
	lowerTitle := strings.ToLower(input.Title)

	return processedArticle{
		ArticleInput:    input,
		normalizedTitle: normalized,
		eventType:       eventType,
		assets:          assets,
		topics:          topics,
		entityIDs:       entityIDs,
		rumor:           isRumorTitle(lowerTitle),
		official:        strings.EqualFold(strings.TrimSpace(input.SourceClass), "official") || isOfficialTitle(lowerTitle),
		numbers:         numbers,
		amountLike:      hasAmountLikeTitle(lowerTitle),
	}
}

func chooseCanonicalArticle(members []processedArticle) processedArticle {
	best := members[0]

	for _, member := range members[1:] {
		if member.SourceWeight > best.SourceWeight {
			best = member
			continue
		}
		if member.SourceWeight < best.SourceWeight {
			continue
		}

		if strings.EqualFold(strings.TrimSpace(member.SourceClass), "official") && !strings.EqualFold(strings.TrimSpace(best.SourceClass), "official") {
			best = member
			continue
		}

		if member.PublishedAt.Before(best.PublishedAt) {
			best = member
		}
	}

	return best
}

func buildSupportingArticles(members []processedArticle) []SupportingArticle {
	sort.Slice(members, func(i, j int) bool {
		return members[i].PublishedAt.Before(members[j].PublishedAt)
	})

	out := make([]SupportingArticle, 0, len(members))
	for _, member := range members {
		out = append(out, SupportingArticle{
			ArticleID:     member.ArticleID,
			Source:        member.SourceName,
			URL:           member.CanonicalURL,
			PublishedAt:   member.PublishedAt.UTC().Format(time.RFC3339),
			EditorialType: member.EditorialType,
		})
	}
	return out
}

func uniqueSourceIDs(members []processedArticle) []string {
	seen := map[string]struct{}{}
	for _, member := range members {
		if member.SourceID == "" {
			continue
		}
		seen[member.SourceID] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for sourceID := range seen {
		out = append(out, sourceID)
	}
	sort.Strings(out)
	return out
}

func hasOfficialSource(members []processedArticle) bool {
	for _, member := range members {
		if strings.EqualFold(strings.TrimSpace(member.SourceClass), "official") {
			return true
		}
	}
	return false
}

func hasUnresolvedConflict(members []processedArticle) bool {
	for i := 0; i < len(members); i++ {
		for j := i + 1; j < len(members); j++ {
			if violatesMergeGuards(members[i], members[j]) {
				return true
			}
		}
	}
	return false
}

func inferStatus(members []processedArticle, uniqueSources int) string {
	if hasOfficialSource(members) {
		return "official_source_confirmed"
	}
	if uniqueSources >= 2 {
		return "multi_source_verified"
	}
	return "single_source"
}

func inferEventTypeForMembers(members []processedArticle) string {
	counts := map[string]int{}
	bestType := ""
	bestCount := 0

	for _, member := range members {
		counts[member.eventType]++
		if counts[member.eventType] > bestCount {
			bestCount = counts[member.eventType]
			bestType = member.eventType
		}
	}

	if bestType == "" {
		return "general_update"
	}
	return bestType
}

func deriveRights(members []processedArticle) Rights {
	modeRank := map[string]int{
		"metadata_only":         0,
		"metadata_plus_summary": 1,
		"metadata_plus_excerpt": 2,
		"full_text_allowed":     3,
	}
	rankMode := map[int]string{
		0: "metadata_only",
		1: "metadata_plus_summary",
		2: "metadata_plus_excerpt",
		3: "full_text_allowed",
	}

	minRank := math.MaxInt
	allExcerptAllowed := true
	for _, member := range members {
		rank, ok := modeRank[member.RightsMode]
		if !ok {
			rank = 0
		}
		if rank < minRank {
			minRank = rank
		}
		if !member.SourceExcerptOK {
			allExcerptAllowed = false
		}
	}
	if minRank == math.MaxInt {
		minRank = 0
	}

	mode := rankMode[minRank]
	excerptAllowed := allExcerptAllowed && (mode == "metadata_plus_excerpt" || mode == "full_text_allowed")

	return Rights{
		StorageMode:    mode,
		FullTextStored: false,
		ExcerptAllowed: excerptAllowed,
	}
}

func maxSourceWeight(members []processedArticle) float64 {
	max := 0.0
	for _, member := range members {
		if member.SourceWeight > max {
			max = member.SourceWeight
		}
	}
	return clamp(max, 0, 1)
}

func recency(publishedAt, now time.Time) float64 {
	if now.Before(publishedAt) {
		return 1
	}
	hours := now.Sub(publishedAt).Hours()
	if hours <= 0 {
		return 1
	}
	return clamp(1-(hours/72.0), 0, 1)
}

func entityImpact(assets, topics []string) float64 {
	if containsAny(assets, []string{"BTC", "ETH"}) {
		return 1
	}
	if len(assets) > 0 {
		return 0.7
	}
	if containsAny(topics, highRelevanceTopics()) {
		return 0.6
	}
	return 0.3
}

func confidenceScore(uniqueSources int, official bool, conflict bool) float64 {
	score := 0.35
	if uniqueSources >= 2 {
		score += 0.20
	}
	if uniqueSources > 2 {
		score += math.Min(float64(uniqueSources-2)*0.10, 0.25)
	}
	if official {
		score += 0.15
	}
	if conflict {
		score -= 0.15
	}
	return clamp(score, 0, 1)
}

func marketRelevanceScore(assets, topics []string, official bool, eventType string) float64 {
	assetProminence := 0.3
	if containsAny(assets, []string{"BTC", "ETH"}) {
		assetProminence = 1.0
	} else if len(assets) > 0 {
		assetProminence = 0.7
	}

	topicClass := 0.5
	if containsAny(topics, highRelevanceTopics()) {
		topicClass = 1.0
	} else if containsAny(topics, lowRelevanceTopics()) {
		topicClass = 0.2
	}

	sourceType := 0.5
	if official {
		sourceType = 0.9
	} else if len(topics) > 0 {
		sourceType = 0.7
	}

	marketMoving := 0.4
	if containsAny([]string{eventType}, highRelevanceTopics()) {
		marketMoving = 1.0
	}

	return clamp((assetProminence+topicClass+sourceType+marketMoving)/4.0, 0, 1)
}

func earliestPublishedAt(members []processedArticle) time.Time {
	earliest := members[0].PublishedAt
	for _, member := range members[1:] {
		if member.PublishedAt.Before(earliest) {
			earliest = member.PublishedAt
		}
	}
	return earliest
}

func earliestFirstSeenAt(members []processedArticle) time.Time {
	earliest := members[0].FirstSeenAt
	for _, member := range members[1:] {
		if member.FirstSeenAt.Before(earliest) {
			earliest = member.FirstSeenAt
		}
	}
	return earliest
}

func mergeEntities(assets []string, members []processedArticle) []Entity {
	entities := make(map[string]Entity)
	for _, asset := range assets {
		key := "asset:" + strings.ToLower(asset)
		entities[key] = Entity{
			Type: "asset",
			ID:   strings.ToLower(asset),
			Name: asset,
		}
	}

	for _, member := range members {
		for _, id := range member.entityIDs {
			entityType, entityName := entityFromID(id)
			key := entityType + ":" + id
			entities[key] = Entity{
				Type: entityType,
				ID:   id,
				Name: entityName,
			}
		}
	}

	out := make([]Entity, 0, len(entities))
	for _, entity := range entities {
		out = append(out, entity)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type == out[j].Type {
			return out[i].ID < out[j].ID
		}
		return out[i].Type < out[j].Type
	})
	return out
}

func entityFromID(id string) (string, string) {
	switch id {
	case "sec":
		return "regulator", "SEC"
	case "kraken":
		return "exchange", "Kraken"
	case "coinbase":
		return "exchange", "Coinbase"
	case "binance":
		return "exchange", "Binance"
	case "blackrock":
		return "company", "BlackRock"
	default:
		return "company", strings.ToUpper(id)
	}
}

func buildEventID(publishedAt time.Time, title string, members []processedArticle) string {
	datePart := publishedAt.UTC().Format("2006_01_02")
	slug := slugify(title)
	if slug == "" {
		slug = "event"
	}
	if len(slug) > 42 {
		slug = slug[:42]
	}

	urls := make([]string, 0, len(members))
	for _, member := range members {
		urls = append(urls, member.CanonicalURL)
	}
	sort.Strings(urls)
	hash := sha1.Sum([]byte(strings.Join(urls, "|")))
	hashPart := hex.EncodeToString(hash[:])[:8]

	return fmt.Sprintf("evt_%s_%s_%s", datePart, slug, hashPart)
}

func slugify(in string) string {
	var b strings.Builder
	lastUnderscore := false

	for _, r := range strings.ToLower(in) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteRune('_')
			lastUnderscore = true
		}
	}

	out := strings.Trim(b.String(), "_")
	return out
}

func normalizeForSimilarity(in string) string {
	lower := strings.ToLower(strings.TrimSpace(in))
	var b strings.Builder
	lastSpace := false

	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == ',' {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if unicode.IsSpace(r) {
			if !lastSpace {
				b.WriteRune(' ')
				lastSpace = true
			}
			continue
		}
		if !lastSpace {
			b.WriteRune(' ')
			lastSpace = true
		}
	}

	return strings.TrimSpace(b.String())
}

func titleSimilarity(a, b string) float64 {
	trigramsA := trigrams(a)
	trigramsB := trigrams(b)

	if len(trigramsA) == 0 || len(trigramsB) == 0 {
		if a == b {
			return 1
		}
		return 0
	}

	countA := make(map[string]int)
	countB := make(map[string]int)

	for _, tri := range trigramsA {
		countA[tri]++
	}
	for _, tri := range trigramsB {
		countB[tri]++
	}

	intersection := 0
	for tri, count := range countA {
		if other, ok := countB[tri]; ok {
			if count < other {
				intersection += count
			} else {
				intersection += other
			}
		}
	}

	return clamp((2*float64(intersection))/float64(len(trigramsA)+len(trigramsB)), 0, 1)
}

func trigrams(s string) []string {
	normalized := strings.ReplaceAll(strings.TrimSpace(s), " ", "_")
	if len(normalized) < 3 {
		return []string{normalized}
	}
	out := make([]string, 0, len(normalized)-2)
	for i := 0; i <= len(normalized)-3; i++ {
		out = append(out, normalized[i:i+3])
	}
	return out
}

func extractAssets(title string) []string {
	upper := strings.ToUpper(title)
	found := map[string]struct{}{}

	assetPatterns := map[string][]string{
		"BTC":  {" BTC ", "BITCOIN", "XBT"},
		"ETH":  {" ETH ", "ETHEREUM"},
		"SOL":  {" SOL ", "SOLANA"},
		"XRP":  {" XRP ", "RIPPLE"},
		"ADA":  {" ADA ", "CARDANO"},
		"BNB":  {" BNB ", "BINANCE COIN"},
		"DOGE": {" DOGE ", "DOGECOIN"},
	}

	padded := " " + upper + " "
	for asset, patterns := range assetPatterns {
		for _, pattern := range patterns {
			if strings.Contains(padded, pattern) {
				found[asset] = struct{}{}
				break
			}
		}
	}

	out := make([]string, 0, len(found))
	for asset := range found {
		out = append(out, asset)
	}
	sort.Strings(out)
	return out
}

func inferTopics(title string) []string {
	lower := strings.ToLower(title)
	topics := make(map[string]struct{})

	contains := func(words ...string) bool {
		for _, word := range words {
			if strings.Contains(lower, word) {
				return true
			}
		}
		return false
	}

	if contains("etf") {
		topics["etf"] = struct{}{}
	}
	if contains("sec", "regulation", "regulatory", "law", "policy", "compliance") {
		topics["policy"] = struct{}{}
	}
	if contains("lawsuit", "charged", "charges", "settlement", "enforcement") {
		topics["enforcement"] = struct{}{}
	}
	if contains("hack", "exploit", "breach", "attack") {
		topics["security_incident"] = struct{}{}
	}
	if contains("listing", "lists", "list on") {
		topics["listing"] = struct{}{}
	}
	if contains("delisting", "delist") {
		topics["delisting"] = struct{}{}
	}
	if contains("flow", "inflow", "outflow") {
		topics["capital_flows"] = struct{}{}
	}
	if contains("exchange", "trading platform") {
		topics["exchange"] = struct{}{}
	}
	if contains("opinion", "commentary") {
		topics["opinion"] = struct{}{}
	}
	if contains("career", "hiring", "joins as", "appointed") {
		topics["career"] = struct{}{}
	}
	if contains("brand", "sponsorship", "marketing") {
		topics["general_brand"] = struct{}{}
	}
	if contains("culture", "community", "meme") {
		topics["culture"] = struct{}{}
	}

	out := make([]string, 0, len(topics))
	for topic := range topics {
		out = append(out, topic)
	}
	sort.Strings(out)
	return out
}

func inferEventType(title string) string {
	topics := inferTopics(title)
	if len(topics) == 0 {
		return "general_update"
	}
	return topics[0]
}

func extractEntityIDs(title string) []string {
	lower := strings.ToLower(title)
	entities := map[string]struct{}{}

	candidates := []string{"sec", "kraken", "coinbase", "binance", "blackrock"}
	for _, candidate := range candidates {
		if strings.Contains(lower, candidate) {
			entities[candidate] = struct{}{}
		}
	}

	out := make([]string, 0, len(entities))
	for entityID := range entities {
		out = append(out, entityID)
	}
	sort.Strings(out)
	return out
}

func isRumorTitle(lowerTitle string) bool {
	keywords := []string{"rumor", "speculation", "unconfirmed", "reportedly"}
	for _, keyword := range keywords {
		if strings.Contains(lowerTitle, keyword) {
			return true
		}
	}
	return false
}

func isOfficialTitle(lowerTitle string) bool {
	keywords := []string{"official", "announces", "approved", "approval", "press release", "files"}
	for _, keyword := range keywords {
		if strings.Contains(lowerTitle, keyword) {
			return true
		}
	}
	return false
}

func hasAmountLikeTitle(lowerTitle string) bool {
	keywords := []string{"$", "million", "billion", "m ", "bn", "inflow", "outflow", "raises", "funding"}
	for _, keyword := range keywords {
		if strings.Contains(lowerTitle, keyword) {
			return true
		}
	}
	return false
}

func extractNumberTokens(title string) []string {
	raw := numberRegexp.FindAllString(title, -1)
	if len(raw) == 0 {
		return nil
	}

	out := make([]string, 0, len(raw))
	for _, token := range raw {
		normalized := strings.ReplaceAll(token, ",", "")
		out = append(out, normalized)
	}
	return out
}

func highRelevanceTopics() []string {
	return []string{
		"policy",
		"enforcement",
		"etf",
		"exchange",
		"security_incident",
		"listing",
		"delisting",
		"capital_flows",
	}
}

func lowRelevanceTopics() []string {
	return []string{
		"opinion",
		"career",
		"general_brand",
		"culture",
	}
}

func flattenAssets(members []processedArticle) []string {
	out := make([]string, 0)
	for _, member := range members {
		out = append(out, member.assets...)
	}
	return out
}

func flattenTopics(members []processedArticle) []string {
	out := make([]string, 0)
	for _, member := range members {
		out = append(out, member.topics...)
	}
	return out
}

func uniqueSorted(items []string) []string {
	set := make(map[string]struct{})
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func intersects(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, item := range a {
		set[item] = struct{}{}
	}
	for _, item := range b {
		if _, ok := set[item]; ok {
			return true
		}
	}
	return false
}

func containsAny(values, targets []string) bool {
	if len(values) == 0 || len(targets) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	for _, target := range targets {
		if _, ok := set[target]; ok {
			return true
		}
	}
	return false
}

func clamp(value, min, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func absDuration(duration time.Duration) time.Duration {
	if duration < 0 {
		return -duration
	}
	return duration
}
