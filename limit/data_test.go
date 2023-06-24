package limit_test

import (
	"testing"

	"github.com/square/finch/limit"
)

func TestOr(t *testing.T) {
	r1 := limit.NewRows(100, 0)
	r2 := limit.NewRows(50, 0)

	dl := limit.Or(r1, r2)

	// We can pass nil for the *sql.DB because Rows doesn't use it for More
	if dl.More(nil) == false {
		t.Error("More false, expected true before anything called")
	}

	r1.Affected(1) // 1/100
	r2.Affected(1) // 1/50
	if dl.More(nil) != true {
		t.Error("More false, expected true before either limit reached")
	}

	r2.Affected(49) // 50/50
	if dl.More(nil) != false {
		t.Error("More true, expected false when one limit reached")
	}
}
