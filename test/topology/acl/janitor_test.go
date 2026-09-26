package acl

import (
	"os"
	"strings"
	"testing"
)

// TestACLJanitor removes this slot's leftover ACLs and MACIP ACLs from the shared VPP (tags "<prefix>:…" and
// "<prefix>-…:…", e.g. after an aborted run): every binding that lists one is rewritten without it (other owners'
// entries kept, D-066) BEFORE the ACL is deleted (D-095c); a MACIP ACL is unbound first. Nothing else is touched.
//
//	VRX_ACL_JANITOR=1 run.sh -run TestACLJanitor
func TestACLJanitor(t *testing.T) {
	if os.Getenv("VRX_INTEGRATION") != "1" || os.Getenv("VRX_ACL_JANITOR") != "1" {
		t.Skip("janitor: set VRX_INTEGRATION=1 and VRX_ACL_JANITOR=1")
	}
	s := slotFromEnv(t)
	sharedLock(t)
	conn := connectVPP(t)
	mine := func(tag string) bool {
		return strings.HasPrefix(tag, s.prefix+":") || strings.HasPrefix(tag, s.prefix+"-")
	}
	ours := map[uint32]string{}
	for _, a := range aclDump(t, conn) {
		if mine(a.tag) {
			ours[a.idx] = a.tag
		}
	}
	for name, i := range dumpIfs(t, conn) {
		n, acls := ifaceACLs(t, conn, i.idx)
		var keep []uint32
		in := n
		for k, x := range acls {
			if _, ok := ours[x]; ok {
				if k < int(n) {
					in--
				}
				continue
			}
			keep = append(keep, x)
		}
		if len(keep) != len(acls) {
			setIfaceACLs(t, conn, i.idx, in, keep...)
			t.Logf("unbound %v from %s (sw_if_index %d), kept %v", acls, name, i.idx, keep)
		}
	}
	// D-071 (review L8): re-read the identity of each index immediately before the delete by index
	now := map[uint32]string{}
	for _, a := range aclDump(t, conn) {
		now[a.idx] = a.tag
	}
	for idx, tag := range ours {
		if now[idx] != tag {
			t.Logf("acl %d is no longer %q (now %q): not deleted", idx, tag, now[idx])
			continue
		}
		delACL(t, conn, idx)
		t.Logf("acl_del %d (%s)", idx, tag)
	}
	macs := map[uint32]string{}
	for _, m := range macipDump(t, conn) {
		if mine(m.tag) {
			macs[m.idx] = m.tag
		}
	}
	for _, b := range macipBindings(t, conn) {
		if _, ok := macs[b.acl]; ok {
			macipUnbind(t, conn, b.swif, b.acl)
			t.Logf("macip unbind %d from sw_if_index %d", b.acl, b.swif)
		}
	}
	nowMacip := map[uint32]string{}
	for _, m := range macipDump(t, conn) {
		nowMacip[m.idx] = m.tag
	}
	for idx, tag := range macs {
		if nowMacip[idx] != tag {
			t.Logf("MACIP acl %d is no longer %q: not deleted", idx, tag)
			continue
		}
		macipDel(t, conn, idx)
		t.Logf("macip_acl_del %d (%s)", idx, tag)
	}
	t.Logf("left for %s: ACLs %v, MACIP %v", s.prefix, ownACLs(t, conn, s.prefix), ownMacips(t, conn, s.prefix))
}
