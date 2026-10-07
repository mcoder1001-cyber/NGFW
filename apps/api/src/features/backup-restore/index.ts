import { BackupRestoreController } from './backup-restore.controller.js';
import { BackupRestoreService } from './backup-restore.service.js';
import { BackupScheduleService } from './schedule.js';
export const backupRestoreFeature = {
  controllers: [BackupRestoreController],
  providers: [BackupRestoreService, BackupScheduleService],
};
