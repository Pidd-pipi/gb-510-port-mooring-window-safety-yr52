
import { request } from './client';
import type { BerthConflictPreview, BerthOccupancy, DomainRecord } from '../types/domain';

export async function listMooringPlan(page = 1, pageSize = 20, search = '') {
  return request<DomainRecord[]>(`/plans?page=${page}&pageSize=${pageSize}&search=${encodeURIComponent(search)}`);
}
export async function createMooringPlan(input: Partial<DomainRecord>) {
  return request<DomainRecord>('/plans', { method: 'POST', body: JSON.stringify(input) });
}
export async function transitionMooringPlan(id: number, status: string, expectedVersion: number, reason: string) {
  return request<DomainRecord>(`/plans/${id}/transition`, {
    method: 'POST', body: JSON.stringify({ status, expectedVersion, reason }),
  });
}

export interface ApprovalInput {
  berthCode: string;
  startAt: string;
  endAt: string;
  windowCode: string;
}

// 方案提交批准：携带泊位、靠泊起止时间和关联风浪窗口；后端在窗口安全且
// 泊位时段空闲时才批准，否则返回业务错误且不留下占用。
export async function approveMooringPlan(id: number, expectedVersion: number, reason: string, input: ApprovalInput) {
  return request<DomainRecord>(`/plans/${id}/transition`, {
    method: 'POST',
    body: JSON.stringify({
      status: 'approved', expectedVersion, reason,
      berthCode: input.berthCode, startAt: input.startAt, endAt: input.endAt, windowCode: input.windowCode,
    }),
  });
}

export async function listBerthOccupancies(params: { page?: number; pageSize?: number; status?: string; berth?: string; planId?: number } = {}) {
  const search = new URLSearchParams();
  search.set('page', String(params.page ?? 1));
  search.set('pageSize', String(params.pageSize ?? 50));
  if (params.status) search.set('status', params.status);
  if (params.berth) search.set('berth', params.berth);
  if (params.planId) search.set('planId', String(params.planId));
  return request<BerthOccupancy[]>(`/berth-occupancies?${search.toString()}`);
}

export async function previewBerthConflicts(input: { berthCode: string; startAt: string; endAt: string }) {
  const search = new URLSearchParams({
    berth: input.berthCode, startAt: input.startAt, endAt: input.endAt,
  });
  return request<BerthConflictPreview>(`/berth-occupancies/conflicts?${search.toString()}`);
}
