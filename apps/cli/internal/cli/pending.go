package cli

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"ngfw/cli/internal/api"
)

// pendingCommit is an applied but not yet confirmed commit (review M3): shown before every prompt with its countdown,
// it blocks the first exit/quit, and its automatic revert is reported.
type pendingCommit struct {
	TxnID    string
	Comment  string
	Deadline time.Time
}

func shortTxn(id string) string { return short(id, 8) }

// whenLocal is "15:04:05 -0700 (in 42 s)" for a deadline.
func whenLocal(t time.Time) string {
	left := time.Until(t).Round(time.Second)
	if left < 0 {
		return t.Local().Format("15:04:05 -0700") + " (passed)"
	}
	return fmt.Sprintf("%s (in %d s)", t.Local().Format("15:04:05 -0700"), int(left.Seconds()))
}

// pendingNote is the one-line reminder.
func (a *App) pendingNote() string {
	p := a.pending
	if p == nil {
		return ""
	}
	return fmt.Sprintf("commit %s is applied but NOT confirmed: it reverts at %s — type `confirm` to keep it", shortTxn(p.TxnID), whenLocal(p.Deadline))
}

// setPending records the answer of a commit/rollback/confirm.
func (a *App) setPending(status, txn, deadline, comment string) {
	if status != "pending" {
		a.pending = nil
		return
	}
	d, err := time.Parse(time.RFC3339Nano, deadline)
	if err != nil {
		d = time.Now()
	}
	a.pending = &pendingCommit{TxnID: txn, Comment: comment, Deadline: d}
}

// refreshPending asks the API (GET /config/commit/pending) and reports a commit that stopped being pending:
// confirmed (its txn is the newest revision) or reverted automatically.
func (a *App) refreshPending(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var out struct {
		Pending *struct {
			TxnID    string `json:"txnId"`
			Comment  string `json:"comment"`
			Deadline string `json:"deadline"`
		} `json:"pending"`
	}
	if _, err := a.call(ctx, api.Call{Op: "Config_pending"}, &out); err != nil {
		return // keep what we knew; the next prompt tries again
	}
	prev := a.pending
	if out.Pending != nil {
		a.setPending("pending", out.Pending.TxnID, out.Pending.Deadline, out.Pending.Comment)
		return
	}
	a.pending = nil
	if prev == nil {
		return
	}
	var revs struct {
		Items []revisionMeta `json:"items"`
	}
	if _, err := a.call(ctx, api.Call{Op: "Config_revisions", Query: url.Values{"limit": {"1"}}}, &revs); err == nil &&
		len(revs.Items) > 0 && revs.Items[0].TxnID != nil && *revs.Items[0].TxnID == prev.TxnID {
		fmt.Fprintf(a.Stdout, "note: commit %s was confirmed (revision %d)\n", shortTxn(prev.TxnID), revs.Items[0].ID)
		return
	}
	fmt.Fprintf(a.Stdout, "note: commit %s was NOT confirmed and has been reverted automatically (deadline %s)\n", shortTxn(prev.TxnID), prev.Deadline.Local().Format("15:04:05 -0700"))
}
