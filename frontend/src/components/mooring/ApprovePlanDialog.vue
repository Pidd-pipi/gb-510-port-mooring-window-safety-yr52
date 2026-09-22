<script setup lang="ts">
import { computed, watch } from 'vue';
import type { DomainRecord, OccupancyCheckResult, OccupancyConflictDetail } from '../../types/domain';
import { formatDate } from '../../utils/format';

export interface ApproveDraft {
  berth: string;
  range: string[];
  windowCode: string;
  windowVersion: number;
  reason: string;
}

const props = defineProps<{
  modelValue: boolean;
  item: DomainRecord | null;
  windows: DomainRecord[];
  form: ApproveDraft;
  submitting?: boolean;
  conflict: OccupancyConflictDetail | null;
  serverError: string;
  checkResult: OccupancyCheckResult | null;
}>();
const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  'update:form': [value: ApproveDraft];
  submit: [];
  precheck: [];
}>();

const BERTH_PRESETS = ['B-01', 'B-02', 'B-03', 'B-04', 'B-05'];

const safeWindows = computed(() => props.windows.filter((item) => item.status === 'safe'));
const canSubmit = computed(() => Boolean(
  props.form.berth.trim()
  && props.form.range.length === 2
  && props.form.windowCode
  && props.form.windowVersion > 0
  && props.form.reason.trim().length >= 3,
));

function patch(patch: Partial<ApproveDraft>) {
  emit('update:form', { ...props.form, ...patch });
}

watch(() => props.modelValue, (open) => {
  if (open && props.item) {
    emit('update:form', {
      berth: props.item.berth || '',
      range: props.item.berthStartAt && props.item.berthEndAt
        ? [props.item.berthStartAt, props.item.berthEndAt]
        : [],
      windowCode: props.item.windowCode || '',
      windowVersion: props.item.windowVersion || 1,
      reason: '',
    });
  }
});

watch(() => props.form.windowCode, (code) => {
  const match = props.windows.find((item) => item.code === code);
  if (match && props.form.windowVersion !== match.version) {
    patch({ windowVersion: match.version });
  }
});

function close() {
  emit('update:modelValue', false);
}
</script>

<template>
  <el-dialog :model-value="modelValue" title="提交批准：填写泊位时段与风浪窗口" width="560px" @close="close">
    <el-alert
      v-if="serverError"
      :title="serverError"
      :type="conflict ? 'warning' : 'error'"
      show-icon
      :closable="false"
      style="margin-bottom: 12px"
    />
    <el-alert
      v-if="conflict"
      type="error"
      :closable="false"
      style="margin-bottom: 12px"
      title="同一泊位存在时段重叠，方案保持原状态且不产生占用"
    >
      <div v-for="slot in conflict.conflicts" :key="`${slot.planCode}-${slot.startAt}`">
        冲突方案 <strong>{{ slot.planCode }}</strong>：{{ slot.berth }}
        {{ formatDate(slot.startAt) }} ~ {{ formatDate(slot.endAt) }}
      </div>
    </el-alert>
    <el-alert
      v-if="checkResult"
      :type="checkResult.available ? 'success' : 'warning'"
      :closable="false"
      style="margin-bottom: 12px"
      :title="checkResult.available ? '预检通过：窗口安全且时段空闲' : '预检未通过'"
    >
      <div>窗口 {{ checkResult.windowCode }} 状态：{{ checkResult.windowStatus || '未找到' }}；重叠占用 {{ checkResult.conflicts.length }} 处</div>
    </el-alert>
    <el-form label-width="96px" @submit.prevent>
      <el-form-item label="泊位" required>
        <el-select :model-value="form.berth" filterable allow-create placeholder="选择或输入泊位编号" style="width: 100%" @update:model-value="(value: string) => patch({ berth: value })">
          <el-option v-for="berth in BERTH_PRESETS" :key="berth" :label="berth" :value="berth"/>
        </el-select>
      </el-form-item>
      <el-form-item label="靠泊起止" required>
        <el-date-picker
          :model-value="form.range"
          type="datetimerange"
          range-separator="至"
          start-placeholder="靠泊开始"
          end-placeholder="靠泊结束"
          format="YYYY-MM-DD HH:mm"
          value-format="YYYY-MM-DDTHH:mm:ss.SSSZ"
          style="width: 100%"
          @update:model-value="(value: string[]) => patch({ range: value || [] })"
        />
      </el-form-item>
      <el-form-item label="风浪窗口" required>
        <el-select :model-value="form.windowCode" placeholder="仅可关联安全窗口" style="width: 100%" @update:model-value="(value: string) => patch({ windowCode: value })">
          <el-option
            v-for="window in safeWindows"
            :key="window.id"
            :label="`${window.code} · ${window.name}（v${window.version}）`"
            :value="window.code"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="窗口版本" required>
        <el-input-number :model-value="form.windowVersion" :min="1" :max="9999" @update:model-value="(value: number) => patch({ windowVersion: value })"/>
      </el-form-item>
      <el-form-item label="批准理由" required>
        <el-input :model-value="form.reason" type="textarea" :rows="2" maxlength="500" show-word-limit @update:model-value="(value: string) => patch({ reason: value })"/>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="close">取消</el-button>
      <el-button :loading="submitting" @click="emit('precheck')">预检占用</el-button>
      <el-button type="primary" :disabled="!canSubmit" :loading="submitting" @click="emit('submit')">提交批准</el-button>
    </template>
  </el-dialog>
</template>
