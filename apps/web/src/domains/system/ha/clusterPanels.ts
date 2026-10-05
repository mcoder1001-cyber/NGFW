import { StateSyncPanel } from '../ha-state-sync/Panel';
import type { ComponentType } from 'react';
/** Additive extension point for F-ha-state-sync. */
export const clusterPanels: readonly ComponentType[] = [StateSyncPanel];
