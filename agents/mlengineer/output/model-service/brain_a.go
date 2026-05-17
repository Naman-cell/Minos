package main

import (
	"database/sql"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"

	_ "modernc.org/sqlite"
)

const embeddingDims = 64

type BrainA struct {
	db *sql.DB
}

type Memory struct {
	Text  string
	Score float64
}

func NewBrainA(path string) (*BrainA, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`create table if not exists memories(
		id integer primary key autoincrement,
		text text not null,
		language text not null default 'auto',
		vector text not null,
		created_at datetime default current_timestamp
	)`); err != nil {
		_ = db.Close()
		return nil, err
	}
	_, _ = db.Exec(`alter table memories add column language text not null default 'auto'`)
	return &BrainA{db: db}, nil
}

func (b *BrainA) Close() error {
	return b.db.Close()
}

func (b *BrainA) StoreTurn(text string) error {
	return b.StoreTurnWithLanguage(text, "auto")
}

func (b *BrainA) StoreTurnWithLanguage(text string, language string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if language == "" {
		language = "auto"
	}
	_, err := b.db.Exec(`insert into memories(text, language, vector) values (?, ?, ?)`, text, language, encodeVector(embed(text)))
	return err
}

func (b *BrainA) Recall(query string, k int) ([]Memory, error) {
	rows, err := b.db.Query(`select text, vector from memories order by id desc limit 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	queryVec := embed(query)
	var memories []Memory
	for rows.Next() {
		var text, raw string
		if err := rows.Scan(&text, &raw); err != nil {
			return nil, err
		}
		memories = append(memories, Memory{Text: text, Score: cosine(queryVec, decodeVector(raw))})
	}
	sort.Slice(memories, func(i, j int) bool { return memories[i].Score > memories[j].Score })
	if len(memories) > k {
		memories = memories[:k]
	}
	return memories, rows.Err()
}

func embed(text string) []float64 {
	vec := make([]float64, embeddingDims)
	for _, token := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(token))
		vec[int(h.Sum32())%embeddingDims]++
	}
	var norm float64
	for _, value := range vec {
		norm += value * value
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return vec
	}
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}

func cosine(a, b []float64) float64 {
	var sum float64
	for i := 0; i < len(a) && i < len(b); i++ {
		sum += a[i] * b[i]
	}
	return sum
}

func encodeVector(vec []float64) string {
	parts := make([]string, len(vec))
	for i, v := range vec {
		parts[i] = fmt.Sprintf("%.6f", v)
	}
	return strings.Join(parts, ",")
}

func decodeVector(raw string) []float64 {
	vec := make([]float64, 0, embeddingDims)
	for _, part := range strings.Split(raw, ",") {
		var value float64
		_, _ = fmt.Sscanf(part, "%f", &value)
		vec = append(vec, value)
	}
	return vec
}
