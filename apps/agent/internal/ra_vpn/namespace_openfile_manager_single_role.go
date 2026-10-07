package ravpn

import "context"

type managerDBusSingleRole uint8

const (
	managerDBusSingleSource managerDBusSingleRole = iota + 1
	managerDBusSingleVPP
	managerDBusSinglePublisherExit
)

func fixedManagerDBusSingleRole(kind managerDBusSingleRole) (managerDBusRole, error) {
	switch kind {
	case managerDBusSingleSource:
		return managerDBusSourceRole, nil
	case managerDBusSingleVPP:
		return managerDBusRole{"vpp.service", "/org/freedesktop/systemd1/unit/vpp_2eservice", "Id,MainPID,FragmentPath,DropInPaths,ExecStart"}, nil
	case managerDBusSinglePublisherExit:
		return managerDBusRole{"ngfw-ra-openfile.service", "/org/freedesktop/systemd1/unit/ngfw_2dra_2dopenfile_2eservice", "Id,MainPID,ControlPID,ActiveState,SubState"}, nil
	}
	return managerDBusRole{}, ErrBoundary
}

func validManagerDBusRoles(roles []managerDBusRole) bool {
	if len(roles) == 1 {
		for _, kind := range []managerDBusSingleRole{managerDBusSingleSource, managerDBusSingleVPP, managerDBusSinglePublisherExit} {
			role, err := fixedManagerDBusSingleRole(kind)
			if err == nil && roles[0] == role {
				return true
			}
		}
		return false
	}
	if len(roles) < 2 || len(roles) > 3 {
		return false
	}
	for i, role := range roles {
		want := managerDBusSourceRole
		if i < 2 {
			want = managerDBusPublisherRoles[i]
		}
		if role != want {
			return false
		}
	}
	return true
}

func readManagerDBusSingleRole(ctx context.Context, kind managerDBusSingleRole) (map[string]string, error) {
	role, err := fixedManagerDBusSingleRole(kind)
	if err != nil {
		return nil, ErrBoundary
	}
	values, err := readManagerDBusRoles(ctx, []managerDBusRole{role})
	if err != nil || len(values) != 1 {
		return nil, ErrBoundary
	}
	return values[0], nil
}
