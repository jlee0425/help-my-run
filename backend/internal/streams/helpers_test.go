package streams

// f64p / strp are pointer helpers for table-driven tests (M3.2.1).
// Same signatures as store_test.go's f64p/strp; defined here because the
// streams test package has its own scope (those store helpers are not visible).
func f64p(v float64) *float64 { return &v }
func strp(v string) *string   { return &v }
