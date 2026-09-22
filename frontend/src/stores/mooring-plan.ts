import { defineStore } from 'pinia';
import {
  approveMooringPlan,
  checkMooringPlanOccupancy,
  listBerthOccupancies,
  transitionMooringPlan,
  type ApprovePlanInput,
} from '../api/mooring-plan';
import { request } from '../api/client';
import { ApiError } from '../api/client';
import type { BerthOccupancy, DomainRecord, OccupancyCheckResult, OccupancyConflictDetail, PageMeta } from '../types/domain';

interface OccupancyState {
  active: BerthOccupancy[];
  released: BerthOccupancy[];
  activeTotal: number;
  releasedTotal: number;
}

export const useMooringPlanStore = defineStore('mooringPlan', {
  state: () => ({
    items: [] as DomainRecord[],
    meta: { page: 1, pageSize: 20, total: 0 } as PageMeta,
    loading: false,
    error: '',
    lastConflict: null as OccupancyConflictDetail | null,
    occupancy: { active: [], released: [], activeTotal: 0, releasedTotal: 0 } as OccupancyState,
  }),
  actions: {
    async load(path: string, search = '') {
      this.loading = true;
      this.error = '';
      try {
        const result = await request<DomainRecord[]>(`/${path}?page=1&pageSize=20&search=${encodeURIComponent(search)}`);
        this.items = result.data;
        this.meta = result.meta || { page: 1, pageSize: 20, total: result.data.length };
        await this.loadOccupancies();
      } catch (error) {
        this.error = error instanceof Error ? error.message : String(error);
      } finally {
        this.loading = false;
      }
    },
    async createRecord(path: string, input: Partial<DomainRecord>) {
      this.loading = true;
      try {
        await request<DomainRecord>(`/${path}`, { method: 'POST', body: JSON.stringify(input) });
        await this.load(path);
      } finally {
        this.loading = false;
      }
    },
    async loadOccupancies() {
      try {
        const [active, released] = await Promise.all([
          listBerthOccupancies({ status: 'active', pageSize: 50 }),
          listBerthOccupancies({ status: 'released', pageSize: 10 }),
        ]);
        this.occupancy.active = active.data;
        this.occupancy.released = released.data;
        this.occupancy.activeTotal = Number(active.meta?.total ?? active.data.length);
        this.occupancy.releasedTotal = Number(released.meta?.total ?? released.data.length);
      } catch (error) {
        this.error = error instanceof Error ? error.message : String(error);
      }
    },
    async transition(path: string, item: DomainRecord, status: string, reason = '前端工作台人工确认') {
      this.loading = true;
      this.error = '';
      try {
        await transitionMooringPlan(item.id, status, item.version, reason);
        await this.load(path);
      } catch (error) {
        this.error = error instanceof Error ? error.message : String(error);
      } finally {
        this.loading = false;
      }
    },
    async approve(path: string, item: DomainRecord, input: ApprovePlanInput): Promise<boolean> {
      this.loading = true;
      this.error = '';
      this.lastConflict = null;
      try {
        await approveMooringPlan(item.id, input);
        await this.load(path);
        return true;
      } catch (error) {
        this.error = error instanceof Error ? error.message : String(error);
        if (error instanceof ApiError && error.code === 'berth_slot_conflict') {
          this.lastConflict = error.details as OccupancyConflictDetail;
        }
        return false;
      } finally {
        this.loading = false;
      }
    },
    async checkOccupancy(item: DomainRecord, slot: Omit<ApprovePlanInput, 'expectedVersion' | 'reason'>): Promise<OccupancyCheckResult | null> {
      try {
        const response = await checkMooringPlanOccupancy(item.id, slot);
        return response.data.result;
      } catch (error) {
        this.error = error instanceof Error ? error.message : String(error);
        return null;
      }
    },
    async confirmClearance(path: string, item: DomainRecord) {
      await this.transition(path, item, 'cleared', '已复核风浪窗口版本、缆绳方案和回退措施');
    },
  },
});
