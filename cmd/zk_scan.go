package cmd

import (
	"fmt"
	"math"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/dagistankaradeniz/gcp-note-taking-cli/internal/client"
)

// Zero-Knowledge accounts: the server holds tags and titles only as
// ciphertext, so what it would normally aggregate (tag lists, tag filters,
// graph titles, related notes) is worked out here from the account's own
// notes, decrypted on this device -- the same data the web keeps in memory.
// One scan reads at most zkScanMaxNotes notes; past that the result says
// it's partial.

const (
	zkScanPageSize = 200
	zkScanMaxNotes = 5_000
)

// zkScan is every active note (decrypted) plus canvas titles, read once.
type zkScan struct {
	notes     []client.Note
	canvases  map[string]string
	truncated bool
}

func scanZk(c *client.Client, dek []byte, withCanvases bool) (*zkScan, error) {
	s := &zkScan{canvases: map[string]string{}}
	cursor := ""
	for {
		q := url.Values{}
		q.Set("limit", strconv.Itoa(zkScanPageSize))
		if cursor != "" {
			q.Set("start_after", cursor)
		}
		var resp client.NoteListResponse
		if err := c.Do("GET", "/v1/notes", q, nil, &resp); err != nil {
			return nil, err
		}
		for i := range resp.Notes {
			decryptNoteZK(dek, &resp.Notes[i])
		}
		s.notes = append(s.notes, resp.Notes...)
		if !resp.HasMore || len(resp.Notes) == 0 {
			break
		}
		if len(s.notes) >= zkScanMaxNotes {
			s.truncated = true
			break
		}
		cursor = resp.Notes[len(resp.Notes)-1].ID
	}
	if withCanvases {
		cursor = ""
		for n := 0; n < zkScanMaxNotes; {
			q := url.Values{}
			q.Set("limit", strconv.Itoa(zkScanPageSize))
			if cursor != "" {
				q.Set("start_after", cursor)
			}
			var resp client.CanvasListResponse
			if err := c.Do("GET", "/v1/canvases", q, nil, &resp); err != nil {
				// No canvas scope (an older login) or no canvas plan:
				// canvas nodes just keep their generic label.
				break
			}
			for i := range resp.Canvases {
				decryptCanvasTitleZK(dek, &resp.Canvases[i])
				s.canvases[resp.Canvases[i].ID] = canvasTitle(resp.Canvases[i])
			}
			n += len(resp.Canvases)
			if !resp.HasMore || len(resp.Canvases) == 0 {
				break
			}
			cursor = resp.Canvases[len(resp.Canvases)-1].ID
		}
	}
	return s, nil
}

func (s *zkScan) warnIfPartial() {
	if s.truncated {
		fmt.Fprintf(os.Stderr, "\nOnly the first %d notes were read, so this result may be incomplete.\n", zkScanMaxNotes)
	}
}

// titles maps note and canvas ids to their decrypted titles.
func (s *zkScan) titles() map[string]string {
	m := make(map[string]string, len(s.notes)+len(s.canvases))
	for id, t := range s.canvases {
		m[id] = t
	}
	for _, n := range s.notes {
		m[n.ID] = orDefault(n.Title, "Untitled note")
	}
	return m
}

// zkGraphOffText is shown when a Zero-Knowledge account hasn't turned on
// the Knowledge Graph (its links aren't shared with the server).
const zkGraphOffText = "Turn on the Knowledge Graph in Settings to use this with a Zero-Knowledge account"

// zkTitleUnknown stands in for an item the scan didn't find.
const zkTitleUnknown = "(not available)"

// fillZkGraphTitles puts decrypted titles on graph nodes: for a ZK account
// the server only ever stores ids in the graph, never titles.
func fillZkGraphTitles(c *client.Client, dek []byte, nodes []client.NoteGraphNode) error {
	if dek == nil || len(nodes) == 0 {
		return nil
	}
	scan, err := scanZk(c, dek, true)
	if err != nil {
		return err
	}
	titles := scan.titles()
	for i := range nodes {
		// Never the server's own text (fail closed, like note titles).
		nodes[i].Title = orDefault(titles[nodes[i].ID], zkTitleUnknown)
		nodes[i].Tags = nil
	}
	scan.warnIfPartial()
	return nil
}

// normalizeTag matches the server's tag_index.normalize for comparisons:
// trimmed, case-insensitive.
func normalizeTag(t string) string { return strings.ToLower(strings.TrimSpace(t)) }

// tagCounts is `tag list --counts` for a ZK account: note counts only (a
// canvas's tags aren't part of the scan).
func (s *zkScan) tagCounts() []client.TagCount {
	counts := map[string]int{}
	names := map[string]string{}
	for _, n := range s.notes {
		seen := map[string]bool{}
		for _, t := range n.Tags {
			k := normalizeTag(t)
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			counts[k]++
			if _, ok := names[k]; !ok {
				names[k] = strings.TrimSpace(t)
			}
		}
	}
	out := make([]client.TagCount, 0, len(counts))
	for k, n := range counts {
		out = append(out, client.TagCount{Name: names[k], Notes: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Notes != out[j].Notes {
			return out[i].Notes > out[j].Notes
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// withTags keeps the notes carrying every (matchAll) or any of tags.
func (s *zkScan) withTags(tags []string, matchAll bool) []client.Note {
	want := map[string]bool{}
	for _, t := range tags {
		want[normalizeTag(t)] = true
	}
	var out []client.Note
	for _, n := range s.notes {
		have := map[string]bool{}
		for _, t := range n.Tags {
			have[normalizeTag(t)] = true
		}
		hits := 0
		for t := range want {
			if have[t] {
				hits++
			}
		}
		if (matchAll && hits == len(want)) || (!matchAll && hits > 0) {
			out = append(out, n)
		}
	}
	return out
}

// Same tuning as the backend's notes_service/related.py.
const (
	relatedSharedLinkWeight = 0.25
	relatedSharedLinkCap    = 0.75
	relatedMinScore         = 0.3
)

// related mirrors find_related_notes over the decrypted notes: jaccard of
// tags plus a capped, degree-weighted shared-link term (Adamic-Adar), with
// anything already linked to or from the note left out. Links come from
// the notes' link lists, which a ZK account only has with the Knowledge
// Graph turned on; without it, ranking is by tags alone. Paths through
// canvases aren't counted.
func (s *zkScan) related(noteID string, limit int) ([]client.NoteRelatedResult, bool) {
	byID := map[string]client.Note{}
	linkedBy := map[string][]string{}
	for _, n := range s.notes {
		byID[n.ID] = n
		for _, target := range n.LinksTo {
			linkedBy[target] = append(linkedBy[target], n.ID)
		}
	}
	source, ok := byID[noteID]
	if !ok {
		return nil, false
	}
	neighbours := func(id string) map[string]bool {
		set := map[string]bool{}
		for _, t := range byID[id].LinksTo {
			set[t] = true
		}
		for _, f := range linkedBy[id] {
			set[f] = true
		}
		delete(set, id)
		return set
	}
	direct := neighbours(noteID)
	excluded := map[string]bool{noteID: true}
	for id := range direct {
		excluded[id] = true
	}

	sharedLinks := map[string]int{}
	strength := map[string]float64{}
	for via := range direct {
		if _, ok := byID[via]; !ok {
			continue
		}
		ids := neighbours(via)
		weight := 2 / math.Log2(2+float64(len(ids)))
		for cid := range ids {
			if !excluded[cid] {
				sharedLinks[cid]++
				strength[cid] += weight
			}
		}
	}

	sourceTags := map[string]bool{}
	for _, t := range source.Tags {
		sourceTags[normalizeTag(t)] = true
	}
	type scored struct {
		res   client.NoteRelatedResult
		links int
	}
	var all []scored
	for _, n := range s.notes {
		if excluded[n.ID] || n.VaultEncrypted || n.Status != "active" {
			continue
		}
		candidate := map[string]bool{}
		for _, t := range n.Tags {
			candidate[normalizeTag(t)] = true
		}
		var shared []string
		unionSize := len(sourceTags)
		for t := range candidate {
			if sourceTags[t] {
				shared = append(shared, t)
			} else {
				unionSize++
			}
		}
		jaccard := 0.0
		if unionSize > 0 {
			jaccard = float64(len(shared)) / float64(unionSize)
		}
		score := jaccard + math.Min(relatedSharedLinkWeight*strength[n.ID], relatedSharedLinkCap)
		if score < relatedMinScore {
			continue
		}
		sort.Strings(shared)
		all = append(all, scored{client.NoteRelatedResult{
			ID: n.ID, Title: orDefault(n.Title, "Untitled note"), Type: "note",
			Score: math.Round(score*1000) / 1000, SharedTags: shared, SharedLinkCount: sharedLinks[n.ID],
		}, sharedLinks[n.ID]})
	}
	sort.SliceStable(all, func(i, j int) bool {
		if all[i].res.Score != all[j].res.Score {
			return all[i].res.Score > all[j].res.Score
		}
		return all[i].links > all[j].links
	})
	out := make([]client.NoteRelatedResult, 0, limit)
	for i := 0; i < len(all) && i < limit; i++ {
		out = append(out, all[i].res)
	}
	return out, true
}
