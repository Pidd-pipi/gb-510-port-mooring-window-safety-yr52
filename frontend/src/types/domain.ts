
export interface DomainRecord {
  id: number;
  code: string;
  name: string;
  status: string;
  version: number;
  description: string;
  facility: string;
  owner: string;
  category: string;
  riskLevel: 'low' | 'medium' | 'high' | 'critical';
  metricValue: number;
  metricUnit: string;
  effectiveAt: string;
  evidence: string;
  relatedCode: string;
  windowVersion?: number;
  submittedBy?: string;
  submittedAt?: string;
  confirmedBy?: string;
  confirmedAt?: string;
  berthCode?: string;
  berthStartAt?: string;
  berthEndAt?: string;
  windowCode?: string;
  currentOccupancyId?: number;
  createdAt: string;
  updatedAt: string;
}

export interface BerthOccupancy {
  id: number;
  code: string;
  status: 'active' | 'released';
  version: number;
  berthCode: string;
  startAt: string;
  endAt: string;
  planId: number;
  planCode: string;
  windowCode: string;
  acquiredBy: string;
  acquiredAt: string;
  releasedBy?: string;
  releasedAt?: string;
  releaseReason?: string;
  createdAt: string;
  updatedAt: string;
}

export interface BerthConflictPreview {
  berth: string;
  startAt: string;
  endAt: string;
  free: boolean;
  conflicts: BerthOccupancy[];
}

export interface PageMeta { page: number; pageSize: number; total: number }
export interface ApiEnvelope<T> { data: T; error?: string; message?: string; meta?: PageMeta }
export interface UserSession { token: string; username: string; displayName: string; role: string; expiresIn: number }
export interface AuditLog {
  id: number; requestId: string; actor: string; action: string; entityType: string;
  entityId: number; beforeState: string; afterState: string; windowVersion?: number; detail: string; createdAt: string;
}
export interface EntityConfig { key: string; path: string; label: string; statuses: readonly string[] }
