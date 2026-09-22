<script setup lang="ts">
import { computed, onMounted, ref } from 'vue';
import EntityPage from '../components/EntityPage.vue';
import RiskBadge from '../components/common/RiskBadge.vue';
import ConfirmDialog from '../components/common/ConfirmDialog.vue';
import ApprovePlanDialog, { type ApproveDraft } from '../components/mooring/ApprovePlanDialog.vue';
import OccupancyPanel from '../components/mooring/OccupancyPanel.vue';
import { ENTITY_CONFIGS } from '../types/status';
import { useMooringPlanStore } from '../stores/mooring-plan';
import { useWeatherWindowStore } from '../stores/weather-window';
import type { ApprovePlanInput } from '../api/mooring-plan';
import type { DomainRecord, OccupancyCheckResult, OccupancyConflictDetail } from '../types/domain';

const store = useMooringPlanStore();
const windowStore = useWeatherWindowStore();

const approveTarget = ref<DomainRecord | null>(null);
const draft = ref<ApproveDraft>({ berth: '', range: [], windowCode: '', windowVersion: 1, reason: '' });
const serverError = ref('');
const conflict = ref<OccupancyConflictDetail | null>(null);
const checkResult = ref<OccupancyCheckResult | null>(null);

const releaseTarget = ref<{ item: DomainRecord; target: 'review' | 'superseded'; title: string } | null>(null);
const releaseReason = ref('');

const approveOpen = computed({
  get: () => approveTarget.value !== null,
  set: (open: boolean) => { if (!open) approveTarget.value = null; },
});

onMounted(() => {
  void windowStore.load('weather-windows');
});

function openApprove(item: DomainRecord) {
  approveTarget.value = item;
  serverError.value = '';
  conflict.value = null;
  checkResult.value = null;
}

function slotPayload() {
  return {
    berth: draft.value.berth.trim(),
    startAt: new Date(draft.value.range[0]).toISOString(),
    endAt: new Date(draft.value.range[1]).toISOString(),
    windowCode: draft.value.windowCode,
    windowVersion: draft.value.windowVersion,
  };
}

async function handlePrecheck() {
  if (!approveTarget.value || draft.value.range.length !== 2) return;
  serverError.value = '';
  const result = await store.checkOccupancy(approveTarget.value, slotPayload());
  checkResult.value = result;
  if (!result) {
    serverError.value = store.error;
  }
}

async function handleSubmit() {
  if (!approveTarget.value || draft.value.range.length !== 2) return;
  serverError.value = '';
  conflict.value = null;
  const payload: ApprovePlanInput = {
    expectedVersion: approveTarget.value.version,
    ...slotPayload(),
    reason: draft.value.reason.trim(),
  };
  const ok = await store.approve('plans', approveTarget.value, payload);
  if (ok) {
    approveTarget.value = null;
    return;
  }
  // 409 berth_slot_conflict 携带结构化冲突时段；方案保持原状态且无占用。
  serverError.value = store.error;
  if (store.lastConflict) {
    conflict.value = store.lastConflict;
  }
}

function askRelease(item: DomainRecord, target: 'review' | 'superseded') {
  releaseTarget.value = {
    item,
    target,
    title: target === 'review' ? '确认撤回已批准方案' : '确认替代已批准方案',
  };
  releaseReason.value = target === 'review' ? '现场作业调整，撤回方案并释放泊位时段' : '方案已被新版本替代，释放原泊位时段';
}

async function confirmRelease() {
  if (!releaseTarget.value) return;
  await store.transition(
    'plans',
    releaseTarget.value.item,
    releaseTarget.value.target,
    releaseReason.value || '前端工作台人工确认',
  );
  releaseTarget.value = null;
}
</script>

<template>
  <EntityPage
    :config="ENTITY_CONFIGS[1]"
    :store="store"
    show-berth-slot
    custom-row-actions
  >
    <template #insight>
      <RiskBadge :records="store.items"/>
      <OccupancyPanel
        :active="store.occupancy.active"
        :released="store.occupancy.released"
        :active-total="store.occupancy.activeTotal"
        :released-total="store.occupancy.releasedTotal"
      />
    </template>

    <template #row-actions="{ row }">
      <template v-if="row.status === 'draft' || row.status === 'review'">
        <el-button link type="primary" @click="openApprove(row)">提交批准</el-button>
      </template>
      <template v-else-if="row.status === 'approved'">
        <el-button link type="warning" @click="askRelease(row, 'review')">撤回</el-button>
        <el-button link type="danger" @click="askRelease(row, 'superseded')">替代</el-button>
      </template>
      <template v-else-if="row.status === 'superseded'">
        <el-button link type="primary" @click="openApprove(row)">重新批准</el-button>
      </template>
      <span v-else class="muted">-</span>
    </template>

    <ApprovePlanDialog
      v-model="approveOpen"
      :item="approveTarget"
      :windows="windowStore.items"
      v-model:form="draft"
      :submitting="store.loading"
      :conflict="conflict"
      :server-error="serverError"
      :check-result="checkResult"
      @submit="handleSubmit"
      @precheck="handlePrecheck"
    />

    <ConfirmDialog
      :model-value="Boolean(releaseTarget)"
      :title="releaseTarget?.title || '确认操作'"
      @update:model-value="(open: boolean) => { if (!open) releaseTarget = null; }"
      @confirm="confirmRelease"
    >
      <p v-if="releaseTarget?.item.occupancy">
        将释放泊位 <strong>{{ releaseTarget.item.occupancy.berth }}</strong>
        （{{ releaseTarget.item.occupancy.planCode }}）的时段占用，并写入释放审计。
      </p>
      <p v-else>该方案当前没有有效占用记录，状态迁移仍会写入审计。</p>
      <el-input v-model="releaseReason" type="textarea" :rows="2" maxlength="500"/>
    </ConfirmDialog>
  </EntityPage>
</template>
