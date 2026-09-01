package quota

import "testing"

func TestProjectQuotaRejectsOverQuotaWrite(t *testing.T) {
	q := NewMemory()
	if err := q.Assign("/staging/token", 10); err != nil {
		t.Fatalf("Assign: %v", err)
	}
	if err := q.Charge("/staging/token", 8); err != nil {
		t.Fatalf("Charge: %v", err)
	}
	if err := q.Charge("/staging/token", 3); err == nil {
		t.Fatal("over-quota write was accepted")
	}
}
