
import { request } from './client';
import type { BerthOccupancy, DomainRecord, OccupancyCheckResult } from '../types/domain';

export interface ApprovePlanInput {
  expectedVersion: number;
  berth: string;
  startAt: string;
  endAt: string;
  windowCode: string;
  windowVersion: number;
  reason: string;
}

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
export async function approveMooringPlan(id: number, input: ApprovePlanInput) {
  return request<DomainRecord>(`/plans/${id}/approve`, { method: 'POST', body: JSON.stringify(input) });
}
export async function checkMooringPlanOccupancy(
  id: number,
  slot: Omit<ApprovePlanInput, 'expectedVersion' | 'reason'>,
) {
  return request<{ planId: number; result: OccupancyCheckResult }>(`/plans/${id}/occupancy-check`, {
    method: 'POST', body: JSON.stringify(slot),
  });
}
export async function listBerthOccupancies(params: { page?: number; pageSize?: number; berth?: string; status?: string } = {}) {
  const query = new URLSearchParams();
  query.set('page', String(params.page ?? 1));
  query.set('pageSize', String(params.pageSize ?? 20));
  if (params.berth) query.set('berth', params.berth);
  if (params.status) query.set('status', params.status);
  return request<BerthOccupancy[]>(`/berth-occupancies?${query.toString()}`);
}
