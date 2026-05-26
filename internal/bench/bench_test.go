package bench

import (
	"testing"

	"github.com/agent-os/agent-os/pkg/context"
)

func BenchmarkContextGeneration(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		eng := context.NewEngine(".")
		_, err := eng.GetSummary()
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkContextCacheHit(b *testing.B) {
	eng := context.NewEngine(".")
	_, err := eng.GetSummary() // warm
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := eng.GetSummary()
		if err != nil {
			b.Fatal(err)
		}
	}
}
