package agent

// IpsecState reports owned native VPP IKE/CHILD SAs without authentication material.
// The product RPC uses native state only and requires the safe-state plugin capability.

import (
	"context"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// IpsecState implements the IpsecState RPC.
func (g *server) IpsecState(ctx context.Context, req *vrxv1.IpsecStateRequest) (*vrxv1.IpsecStateResponse, error) {
	if err := g.svc.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	path := ""
	if reader, ok := g.stats.(*statsReader); ok {
		path = reader.path
	}
	return g.svc.nativeIpsecState(ctx, req, path)
}

// IpsecState builds the response from native VPP state.
func (s *Service) IpsecState(ctx context.Context, req *vrxv1.IpsecStateRequest) (*vrxv1.IpsecStateResponse, error) {
	if err := s.checkOwner(req.GetOwner()); err != nil {
		return nil, err
	}
	return s.nativeIpsecState(ctx, req)
}

// ipsecMaxSAs is the server default and upper bound of IpsecStateRequest.limit.
const ipsecMaxSAs = 1000

// pageSAs applies offset/limit to the IKE_SAs and returns the page and the total before paging.
func pageSAs(sas []*vrxv1.IpsecIkeSa, offset, limit uint32) ([]*vrxv1.IpsecIkeSa, uint32) {
	total := uint32(len(sas)) //nolint:gosec // bounded by charon's SA count
	if limit == 0 || limit > ipsecMaxSAs {
		limit = ipsecMaxSAs
	}
	if offset >= total {
		return nil, total
	}
	end := min(total, offset+limit)
	return sas[offset:end], total
}
