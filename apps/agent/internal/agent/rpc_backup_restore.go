package agent

import (
	"google.golang.org/grpc"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	backuprestore "ngfw/agent/internal/actions/backup-restore"
)

func (g *server) backupRestoreAction(req *ngfwv1.ActionRequest, stream grpc.ServerStreamingServer[ngfwv1.ActionOutput]) error {
	var lines []string
	var err error
	if req.GetUpgrade() != nil {
		lines, err = backuprestore.Upgrade(stream.Context(), req.GetUpgrade(), backuprestore.Runner())
	} else {
		lines, err = backuprestore.Support(stream.Context(), req.GetSupportBundle(), backuprestore.Runner())
	}
	if err != nil {
		return err
	}
	for _, line := range lines {
		if err := stream.Send(&ngfwv1.ActionOutput{Output: &ngfwv1.ActionOutput_Line{Line: line}}); err != nil {
			return err
		}
	}
	return stream.Send(&ngfwv1.ActionOutput{Output: &ngfwv1.ActionOutput_Done{Done: &ngfwv1.ActionDone{Summary: "operation completed", ExitCode: 0}}})
}
