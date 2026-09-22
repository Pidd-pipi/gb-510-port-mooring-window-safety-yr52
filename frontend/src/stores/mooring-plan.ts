import { defineStore } from 'pinia';
import { request } from '../api/client';
import {
  approveMooringPlan, listBerthOccupancies, previewBerthConflicts,
} from '../api/mooring-plan';
import type {
  BerthConflictPreview, BerthOccupancy, DomainRecord, PageMeta,
} from '../types/domain';

interface ApprovalForm {
  berthCode: string;
  startAt: string;
  endAt: string;
  windowCode: string;
}

// 系泊方案仓库在通用实体 CRUD 之外维护泊位占用闭环：占用/释放结论回读、
// 批准前冲突预览，以及携带泊位时段的批准动作。
export const useMooringPlanStore = defineStore('mooringPlan', {
  state: () => ({
    items: [] as DomainRecord[],
    meta: { page: 1, pageSize: 20, total: 0 } as PageMeta,
    occupancies: [] as BerthOccupancy[],
    occupancyMeta: { page: 1, pageSize: 50, total: 0 } as PageMeta,
    loading: false,
    error: '',
    actionError: '',
  }),
  getters: {
    activeOccupancies(state): BerthOccupancy[] {
      return state.occupancies.filter((item) => item.status === 'active');
    },
    releasedOccupancies(state): BerthOccupancy[] {
      return state.occupancies.filter((item) => item.status === 'released');
    },
  },
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
    async loadOccupancies() {
      try {
        const result = await listBerthOccupancies({ pageSize: 50 });
        this.occupancies = result.data;
        this.occupancyMeta = result.meta || { page: 1, pageSize: 50, total: result.data.length };
      } catch (error) {
        this.actionError = error instanceof Error ? error.message : String(error);
      }
    },
    async createRecord(path: string, input: Partial<DomainRecord>) {
      this.loading = true;
      this.actionError = '';
      try {
        await request<DomainRecord>(`/${path}`, { method: 'POST', body: JSON.stringify(input) });
        await this.load(path);
      } catch (error) {
        this.actionError = error instanceof Error ? error.message : String(error);
        throw error;
      } finally {
        this.loading = false;
      }
    },
    // 通用状态迁移：用于 draft -> review、review -> draft 等不涉及泊位占用的动作。
    async transition(path: string, item: DomainRecord, status: string) {
      this.loading = true;
      this.actionError = '';
      try {
        await request<DomainRecord>(`/${path}/${item.id}/transition`, {
          method: 'POST',
          body: JSON.stringify({ status, expectedVersion: item.version, reason: '前端工作台人工确认' }),
        });
        await this.load(path);
      } catch (error) {
        this.actionError = error instanceof Error ? error.message : String(error);
      } finally {
        this.loading = false;
      }
    },
    // 撤回 (approved -> review) 或替代 (approved -> superseded)：后端同步释放占用。
    async release(path: string, item: DomainRecord, status: 'review' | 'superseded', reason: string) {
      this.loading = true;
      this.actionError = '';
      try {
        await request<DomainRecord>(`/${path}/${item.id}/transition`, {
          method: 'POST',
          body: JSON.stringify({ status, expectedVersion: item.version, reason }),
        });
        await this.load(path);
        return true;
      } catch (error) {
        this.actionError = error instanceof Error ? error.message : String(error);
        return false;
      } finally {
        this.loading = false;
      }
    },
    async approve(path: string, item: DomainRecord, form: ApprovalForm, reason: string) {
      this.loading = true;
      this.actionError = '';
      try {
        await approveMooringPlan(item.id, item.version, reason, form);
        await this.load(path);
        return true;
      } catch (error) {
        this.actionError = error instanceof Error ? error.message : String(error);
        return false;
      } finally {
        this.loading = false;
      }
    },
    async previewConflicts(form: ApprovalForm): Promise<BerthConflictPreview | null> {
      if (!form.berthCode || !form.startAt || !form.endAt) return null;
      try {
        const result = await previewBerthConflicts({
          berthCode: form.berthCode.trim().toUpperCase(),
          startAt: new Date(form.startAt).toISOString(),
          endAt: new Date(form.endAt).toISOString(),
        });
        return result.data;
      } catch (error) {
        this.actionError = error instanceof Error ? error.message : String(error);
        return null;
      }
    },
  },
});
