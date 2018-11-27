package main

import (
	"encoding/json"
	"github.com/tystuyfzand/ovrstat/ovrstat"
	"testing"
)

func Test_Stats(t *testing.T) {
	stats, err := ovrstat.PCStats("us", "cats-11481")

	if err != nil {
		t.Fatal(err)
	}

	if stats.Private {
		t.Fatal("Profile shouldn't be private")
	}

	if len(stats.QuickPlayStats.TopHeroes) == 0 {
		t.Fatal("Expected more than zero top heroes")
	}

	if len(stats.QuickPlayStats.CareerStats) == 0 {
		t.Fatal("Expected more than zero career stats")
	} else {
		b, _ := json.MarshalIndent(stats.QuickPlayStats.TopHeroes["ashe"], "", "\t")

		t.Log(string(b))
	}
}