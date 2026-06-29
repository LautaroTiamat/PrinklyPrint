package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func mustCreate(t *testing.T, st *Store, id string, status Status) {
	t.Helper()
	err := st.CreateJob(context.Background(), Job{
		ID: id, Filename: id + ".pdf", PDFPath: "pdfs/" + id + ".pdf.enc", Status: status,
	})
	if err != nil {
		t.Fatalf("CreateJob(%s): %v", id, err)
	}
}

func TestCountByStatus(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	mustCreate(t, st, "q1", StatusQueued)
	mustCreate(t, st, "q2", StatusQueued)
	mustCreate(t, st, "d1", StatusDone)

	if n, err := st.CountByStatus(ctx, StatusQueued); err != nil || n != 2 {
		t.Errorf("CountByStatus(queued) = %d, %v; quiero 2, nil", n, err)
	}
	if n, err := st.CountByStatus(ctx, StatusDone); err != nil || n != 1 {
		t.Errorf("CountByStatus(done) = %d, %v; quiero 1, nil", n, err)
	}
	if n, _ := st.CountByStatus(ctx, StatusFailed); n != 0 {
		t.Errorf("CountByStatus(failed) = %d; quiero 0", n)
	}
}

func TestListJobsFilterAndPagination(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		mustCreate(t, st, fmt.Sprintf("q%d", i), StatusQueued)
	}
	for i := 0; i < 2; i++ {
		mustCreate(t, st, fmt.Sprintf("d%d", i), StatusDone)
	}

	// Filtro por status.
	jobs, total, err := st.ListJobs(ctx, ListJobsFilter{Status: StatusQueued})
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if total != 5 || len(jobs) != 5 {
		t.Errorf("filtro queued: total=%d len=%d; quiero 5 y 5", total, len(jobs))
	}

	// Sin filtro: total = 7.
	if _, total, _ := st.ListJobs(ctx, ListJobsFilter{}); total != 7 {
		t.Errorf("sin filtro: total=%d; quiero 7", total)
	}

	// Paginación: limit acota la página pero total sigue siendo el del filtro.
	page, total, _ := st.ListJobs(ctx, ListJobsFilter{Status: StatusQueued, Limit: 2, Offset: 0})
	if total != 5 || len(page) != 2 {
		t.Errorf("limit=2: total=%d len=%d; quiero 5 y 2", total, len(page))
	}

	// Offset cerca del final: queda 1.
	if last, _, _ := st.ListJobs(ctx, ListJobsFilter{Status: StatusQueued, Limit: 2, Offset: 4}); len(last) != 1 {
		t.Errorf("offset=4: len=%d; quiero 1", len(last))
	}
}

// TestRecoverStaleJobs verifica el recovery tras un crash: los jobs que quedaron
// en "printing" se revierten a "queued" para que el worker los reintente.
func TestRecoverStaleJobs(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	mustCreate(t, st, "p1", StatusPrinting)
	mustCreate(t, st, "p2", StatusPrinting)
	mustCreate(t, st, "q1", StatusQueued)

	n, err := st.RecoverStaleJobs(ctx)
	if err != nil {
		t.Fatalf("RecoverStaleJobs: %v", err)
	}
	if n != 2 {
		t.Errorf("RecoverStaleJobs = %d; quiero 2 (los que estaban printing)", n)
	}

	if c, _ := st.CountByStatus(ctx, StatusPrinting); c != 0 {
		t.Errorf("quedan %d en printing; quiero 0", c)
	}
	if c, _ := st.CountByStatus(ctx, StatusQueued); c != 3 {
		t.Errorf("queued = %d; quiero 3 (2 recuperados + 1 original)", c)
	}

	j, err := st.GetJob(ctx, "p1")
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if j.Status != StatusQueued {
		t.Errorf("p1 status = %s; quiero queued", j.Status)
	}
}

func TestGetJobNotFound(t *testing.T) {
	st := testStore(t)
	if _, err := st.GetJob(context.Background(), "no-existe"); err != ErrNotFound {
		t.Errorf("GetJob inexistente = %v; quiero ErrNotFound", err)
	}
}
