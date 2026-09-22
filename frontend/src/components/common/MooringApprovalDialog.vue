<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import type { DomainRecord } from '../../types/domain';

const props = defineProps<{
  modelValue: boolean;
  plan: DomainRecord | null;
  windows: DomainRecord[];
  loading?: boolean;
}>();
const emit = defineEmits<{
  'update:modelValue': [value: boolean];
  confirm: [payload: { berthCode: string; startAt: string; endAt: string; windowCode: string; reason: string }];
  preview: [form: { berthCode: string; startAt: string; endAt: string; windowCode: string }];
}>();

const safeWindows = computed(() => props.windows.filter((item) => item.status === 'safe'));

const form = reactive({ berthCode: '', startAt: '', endAt: '', windowCode: '' });
const reason = ref('');

watch(() => props.modelValue, (open) => {
  if (open && props.plan) {
    // 回读已固化的泊位时段，替代后重新批准可直接复用。
    form.berthCode = props.plan.berthCode || '';
    form.windowCode = props.plan.windowCode || '';
    form.startAt = props.plan.berthStartAt ? toLocalInput(props.plan.berthStartAt) : '';
    form.endAt = props.plan.berthEndAt ? toLocalInput(props.plan.berthEndAt) : '';
    reason.value = '';
  }
});

function toLocalInput(value: string): string {
  const date = new Date(value);
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

const payload = computed(() => ({
  berthCode: form.berthCode.trim().toUpperCase(),
  startAt: form.startAt ? new Date(form.startAt).toISOString() : '',
  endAt: form.endAt ? new Date(form.endAt).toISOString() : '',
  windowCode: form.windowCode.trim().toUpperCase(),
  reason: reason.value.trim(),
}));

const valid = computed(() =>
  payload.value.berthCode && payload.value.startAt && payload.value.endAt &&
  payload.value.windowCode && payload.value.reason.length >= 3 &&
  new Date(payload.value.endAt) > new Date(payload.value.startAt),
);

function submit() {
  if (!valid.value) return;
  emit('confirm', payload.value);
}

function checkConflicts() {
  if (!payload.value.berthCode || !payload.value.startAt || !payload.value.endAt) return;
  emit('preview', payload.value);
}
</script>

<template>
  <el-dialog :model-value="modelValue" title="提交批准：泊位时段占用" width="560px" @close="emit('update:modelValue', false)">
    <div v-if="plan" class="approval-dialog">
      <p class="muted">方案 <strong>{{ plan.code }}</strong> 当前状态 <strong>{{ plan.status }}</strong>（v{{ plan.version }}）。只有关联风浪窗口安全且泊位时段空闲时才会批准。</p>
      <el-form label-position="top">
        <el-form-item label="泊位" required>
          <el-input v-model="form.berthCode" placeholder="如 B-03" maxlength="64"/>
        </el-form-item>
        <div class="approval-dialog__row">
          <el-form-item label="靠泊开始" required>
            <el-date-picker v-model="form.startAt" type="datetime" placeholder="开始时间" value-format="YYYY-MM-DDTHH:mm:ss" style="width: 100%"/>
          </el-form-item>
          <el-form-item label="靠泊结束" required>
            <el-date-picker v-model="form.endAt" type="datetime" placeholder="结束时间" value-format="YYYY-MM-DDTHH:mm:ss" style="width: 100%"/>
          </el-form-item>
        </div>
        <el-form-item label="关联风浪窗口（须为 safe）" required>
          <el-select v-model="form.windowCode" placeholder="选择安全风浪窗口" style="width: 100%">
            <el-option v-for="window in safeWindows" :key="window.id" :label="`${window.code} · ${window.name}`" :value="window.code"/>
          </el-select>
          <small v-if="safeWindows.length === 0" class="approval-dialog__warn">当前没有 safe 状态的风浪窗口，无法批准。</small>
        </el-form-item>
        <el-form-item label="批准理由" required>
          <el-input v-model="reason" type="textarea" :rows="2" maxlength="500" show-word-limit placeholder="至少 3 个字，将写入审计"/>
        </el-form-item>
      </el-form>
      <slot name="conflicts" :check="checkConflicts"/>
    </div>
    <template #footer>
      <el-button @click="emit('update:modelValue', false)">取消</el-button>
      <el-button :loading="loading" @click="checkConflicts">检查冲突时段</el-button>
      <el-button type="primary" :disabled="!valid" :loading="loading" @click="submit">提交批准</el-button>
    </template>
  </el-dialog>
</template>
