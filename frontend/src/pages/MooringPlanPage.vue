<script setup lang="ts">
import { onMounted, ref } from 'vue';
import EntityPage from '../components/EntityPage.vue';
import RiskBadge from '../components/common/RiskBadge.vue';
import StatusBadge from '../components/common/StatusBadge.vue';
import BerthOccupancyPanel from '../components/common/BerthOccupancyPanel.vue';
import MooringApprovalDialog from '../components/common/MooringApprovalDialog.vue';
import ConfirmDialog from '../components/common/ConfirmDialog.vue';
import { ENTITY_CONFIGS } from '../types/status';
import { useMooringPlanStore } from '../stores/mooring-plan';
import { useWeatherWindowStore } from '../stores/weather-window';
import { useAuth } from '../hooks/useAuth';
import { listBerthOccupancies } from '../api/mooring-plan';
import type { BerthConflictPreview, DomainRecord } from '../types/domain';
import { formatDate } from '../utils/format';
import { computed } from 'vue';

const store = useMooringPlanStore();
const weatherWindowStore = useWeatherWindowStore();
const { session } = useAuth();
const config = ENTITY_CONFIGS[1];

const roleRank: Record<string, number> = { viewer: 1, operator: 2, reviewer: 3, admin: 4 };
const canWrite = computed(() => (roleRank[session.value?.role || ''] || 0) >= roleRank.operator);

const approvalTarget = ref<DomainRecord | null>(null);
const approvalOpen = ref(false);
const releaseTarget = ref<{ item: DomainRecord; status: 'review' | 'superseded' } | null>(null);
const releaseReason = ref('');
const conflictPreview = ref<BerthConflictPreview | null>(null);
const filterPlanId = ref<number | undefined>(undefined);

// EntityPage 挂载时已加载方案与占用列表，这里只补充批准表单需要的风浪窗口。
onMounted(() => {
  void weatherWindowStore.load('weather-windows');
});

function openApproval(item: DomainRecord) {
  conflictPreview.value = null;
  approvalTarget.value = item;
  approvalOpen.value = true;
}

async function submitForReview(item: DomainRecord) {
  await store.transition(config.path, item, 'review');
}

async function checkConflicts(form: { berthCode: string; startAt: string; endAt: string; windowCode: string }) {
  conflictPreview.value = await store.previewConflicts(form);
}

async function submitApproval(form: { berthCode: string; startAt: string; endAt: string; windowCode: string; reason: string }) {
  if (!approvalTarget.value) return;
  const ok = await store.approve(config.path, approvalTarget.value, form, form.reason);
  if (ok) {
    approvalOpen.value = false;
    approvalTarget.value = null;
    conflictPreview.value = null;
  }
}

function askRelease(item: DomainRecord, status: 'review' | 'superseded') {
  releaseReason.value = status === 'superseded' ? '替代已批准方案，回收泊位时段' : '撤回已批准方案，回收泊位时段';
  releaseTarget.value = { item, status };
}

async function confirmRelease() {
  if (!releaseTarget.value || releaseReason.value.trim().length < 3) return;
  const ok = await store.release(config.path, releaseTarget.value.item, releaseTarget.value.status, releaseReason.value.trim());
  if (ok) releaseTarget.value = null;
}

async function viewPlanOccupancies(item: DomainRecord) {
  filterPlanId.value = item.id;
  const result = await listBerthOccupancies({ planId: item.id, pageSize: 20 });
  store.occupancies = result.data;
}

async function showAllOccupancies() {
  filterPlanId.value = undefined;
  await store.loadOccupancies();
}
</script>

<template>
  <EntityPage :config="config" :store="store">
    <template #insight>
      <RiskBadge :records="store.items"/>
      <BerthOccupancyPanel :occupancies="store.occupancies" :plan-id="filterPlanId"/>
      <div v-if="filterPlanId" class="occupancy-filter-bar">
        <el-alert :title="`正在查看方案 #${filterPlanId} 的占用与释放历史`" type="info" show-icon :closable="false"/>
        <el-button size="small" @click="showAllOccupancies">查看全部占用</el-button>
      </div>
      <el-alert v-if="store.actionError" :title="store.actionError" type="error" show-icon :closable="false"/>
    </template>

    <template #table>
      <el-table v-loading="store.loading" :data="store.items">
        <el-table-column prop="code" label="编码" width="120"/>
        <el-table-column label="名称 / 泊位" min-width="190">
          <template #default="{ row }">
            <strong>{{ row.name }}</strong>
            <small>{{ row.berthCode ? `泊位 ${row.berthCode}` : '未分配泊位' }}</small>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="110">
          <template #default="{ row }"><StatusBadge :status="row.status"/></template>
        </el-table-column>
        <el-table-column label="靠泊时段 / 关联窗口" min-width="270">
          <template #default="{ row }">
            <template v-if="row.berthStartAt && row.berthEndAt">
              <small>{{ formatDate(row.berthStartAt) }}</small>
              <small>→ {{ formatDate(row.berthEndAt) }}</small>
              <small>关联窗口：{{ row.windowCode }}</small>
            </template>
            <span v-else class="muted">批准时填写泊位时段</span>
          </template>
        </el-table-column>
        <el-table-column label="占用结论" width="140">
          <template #default="{ row }">
            <span v-if="row.currentOccupancyId" class="status status--success">占用中 #{{ row.currentOccupancyId }}</span>
            <span v-else-if="row.berthCode" class="status status--neutral">已释放</span>
            <span v-else class="muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="owner" label="责任人" width="100"/>
        <el-table-column label="操作" width="300">
          <template #default="{ row }">
            <template v-if="canWrite">
              <el-button v-if="row.status === 'draft'" link @click="submitForReview(row)">送审</el-button>
              <el-button v-if="['draft', 'review', 'superseded'].includes(row.status)" link type="primary" @click="openApproval(row)">提交批准</el-button>
              <el-button v-if="row.status === 'approved'" link type="warning" @click="askRelease(row, 'review')">撤回</el-button>
              <el-button v-if="row.status === 'approved'" link type="danger" @click="askRelease(row, 'superseded')">替代</el-button>
              <el-button link @click="viewPlanOccupancies(row)">占用历史</el-button>
            </template>
            <span v-else class="muted">只读权限</span>
          </template>
        </el-table-column>
      </el-table>
    </template>
  </EntityPage>

  <MooringApprovalDialog
    v-model="approvalOpen"
    :plan="approvalTarget"
    :windows="weatherWindowStore.items"
    :loading="store.loading"
    @confirm="submitApproval"
    @preview="checkConflicts"
  >
    <template #conflicts>
      <div v-if="conflictPreview" class="conflict-box" :class="conflictPreview.free ? 'conflict-box--free' : 'conflict-box--busy'">
        <strong>{{ conflictPreview.free ? '泊位时段空闲，可以提交批准' : `检测到 ${conflictPreview.conflicts.length} 个冲突时段` }}</strong>
        <div v-for="item in conflictPreview.conflicts" :key="item.id" class="conflict-box__row">
          <span>{{ item.planCode }} 已占用 {{ conflictPreview.berth }}</span>
          <small>{{ formatDate(item.startAt) }} → {{ formatDate(item.endAt) }}（窗口 {{ item.windowCode }}，批准人 {{ item.acquiredBy }}）</small>
        </div>
      </div>
    </template>
  </MooringApprovalDialog>

  <ConfirmDialog
    :model-value="Boolean(releaseTarget)"
    :title="releaseTarget?.status === 'superseded' ? '替代已批准方案' : '撤回已批准方案'"
    @update:model-value="(value) => { if (!value) releaseTarget = null; }"
    @confirm="confirmRelease"
  >
    <p>{{ releaseTarget?.status === 'superseded' ? '替代后方案变为 superseded，泊位时段占用立即释放并写入释放结论。' : '撤回后方案回到 review，泊位时段占用立即释放并写入释放结论。' }}</p>
    <el-input v-model="releaseReason" type="textarea" :rows="2" maxlength="500" show-word-limit placeholder="释放原因（至少 3 个字，将写入审计）"/>
  </ConfirmDialog>
</template>
