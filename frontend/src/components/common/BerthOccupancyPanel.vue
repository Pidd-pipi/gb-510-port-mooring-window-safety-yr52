<script setup lang="ts">
import { computed } from 'vue';
import type { BerthOccupancy } from '../../types/domain';
import { formatDate } from '../../utils/format';

const props = defineProps<{ occupancies: BerthOccupancy[]; planId?: number }>();

// 方案页上方的占用闭环面板：左侧为当前占用，右侧为释放结论；传入 planId
// 时额外只看当前方案的占用历史。
const active = computed(() =>
  [...props.occupancies]
    .filter((item) => item.status === 'active' && (props.planId === undefined || item.planId === props.planId))
    .sort((a, b) => a.startAt.localeCompare(b.startAt)),
);
const released = computed(() =>
  [...props.occupancies]
    .filter((item) => item.status === 'released' && (props.planId === undefined || item.planId === props.planId))
    .sort((a, b) => (b.releasedAt || '').localeCompare(a.releasedAt || ''))
    .slice(0, 6),
);

function overlapLabel(item: BerthOccupancy): string {
  return `${formatDate(item.startAt)} → ${formatDate(item.endAt)}`;
}
</script>

<template>
  <section class="occupancy-panel" aria-label="泊位时段占用闭环">
    <header>
      <div>
        <span class="eyebrow">BERTH OCCUPANCY CLOSED LOOP</span>
        <strong>泊位时段占用与释放</strong>
      </div>
      <small>同一泊位时间重叠即冲突；撤回或替代后写入释放结论</small>
    </header>
    <div class="occupancy-columns">
      <article class="occupancy-column occupancy-column--active">
        <h5>当前占用 <em>{{ active.length }}</em></h5>
        <p v-if="active.length === 0" class="muted">暂无已批准方案占用泊位时段。</p>
        <div v-for="item in active" :key="item.id" class="occupancy-row">
          <div class="occupancy-row__title">
            <strong>{{ item.berthCode }}</strong>
            <span class="status status--success">占用中</span>
          </div>
          <dl>
            <dt>方案</dt><dd>{{ item.planCode }}</dd>
            <dt>靠泊时段</dt><dd>{{ overlapLabel(item) }}</dd>
            <dt>关联窗口</dt><dd>{{ item.windowCode }}</dd>
            <dt>批准人</dt><dd>{{ item.acquiredBy }} · {{ formatDate(item.acquiredAt) }}</dd>
          </dl>
        </div>
      </article>
      <article class="occupancy-column occupancy-column--released">
        <h5>释放结论 <em>{{ released.length }}</em></h5>
        <p v-if="released.length === 0" class="muted">尚无撤回或替代释放记录。</p>
        <div v-for="item in released" :key="item.id" class="occupancy-row">
          <div class="occupancy-row__title">
            <strong>{{ item.berthCode }} · {{ item.planCode }}</strong>
            <span class="status status--neutral">已释放</span>
          </div>
          <dl>
            <dt>原占用时段</dt><dd>{{ overlapLabel(item) }}</dd>
            <dt>释放人</dt><dd>{{ item.releasedBy || '-' }} · {{ item.releasedAt ? formatDate(item.releasedAt) : '-' }}</dd>
            <dt>释放结论</dt><dd>{{ item.releaseReason || '方案撤回或替代，泊位时段已回收' }}</dd>
          </dl>
        </div>
      </article>
    </div>
  </section>
</template>
