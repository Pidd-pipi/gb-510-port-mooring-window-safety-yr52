<script setup lang="ts">
import { computed } from 'vue';
import type { BerthOccupancy } from '../../types/domain';
import { formatDate } from '../../utils/format';

const props = defineProps<{
  active: BerthOccupancy[];
  released: BerthOccupancy[];
  activeTotal: number;
  releasedTotal: number;
}>();

const activeRows = computed(() => props.active.slice(0, 5));
const releasedRows = computed(() => props.released.slice(0, 5));
</script>

<template>
  <section class="occupancy-panel" aria-label="泊位时段占用闭环">
    <header>
      <div><span class="eyebrow">BERTH OCCUPANCY</span><strong>泊位时段占用</strong></div>
      <small>批准即占用；撤回或替代已批准方案时释放，结论可刷新回读</small>
    </header>
    <div class="occupancy-columns">
      <article>
        <h4>当前占用（{{ activeTotal }}）</h4>
        <el-table v-if="activeRows.length" :data="activeRows" size="small">
          <el-table-column prop="berth" label="泊位" width="80"/>
          <el-table-column label="占用时段" min-width="200">
            <template #default="{ row }">
              <strong>{{ row.planCode }}</strong>
              <small>{{ formatDate(row.startAt) }} ~ {{ formatDate(row.endAt) }}</small>
            </template>
          </el-table-column>
          <el-table-column label="关联窗口" width="110">
            <template #default="{ row }"><small>{{ row.windowCode }} v{{ row.windowVersion }}</small></template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">暂无有效占用</p>
      </article>
      <article>
        <h4>释放结论（{{ releasedTotal }}）</h4>
        <el-table v-if="releasedRows.length" :data="releasedRows" size="small">
          <el-table-column prop="berth" label="泊位" width="80"/>
          <el-table-column label="释放时段" min-width="170">
            <template #default="{ row }">
              <strong>{{ row.planCode }}</strong>
              <small>{{ formatDate(row.startAt) }} ~ {{ formatDate(row.endAt) }}</small>
            </template>
          </el-table-column>
          <el-table-column label="释放结论" min-width="150">
            <template #default="{ row }">
              <el-tag type="success" size="small">{{ row.releaseReason === 'superseded' ? '替代释放' : (row.releaseReason === 'withdrawn' ? '撤回释放' : '已释放') }}</el-tag>
              <small>{{ row.releasedBy }} · {{ row.releasedAt ? formatDate(row.releasedAt) : '-' }}</small>
            </template>
          </el-table-column>
        </el-table>
        <p v-else class="muted">暂无释放记录</p>
      </article>
    </div>
  </section>
</template>

<style scoped>
.occupancy-panel {
  background: var(--el-bg-color, #fff);
  border: 1px solid var(--el-border-color, #e4e7ed);
  border-radius: 10px;
  padding: 16px 18px;
  margin-bottom: 16px;
}
.occupancy-panel header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  gap: 12px;
  margin-bottom: 10px;
  flex-wrap: wrap;
}
.occupancy-columns {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 14px;
}
.occupancy-columns article {
  border: 1px dashed var(--el-border-color, #dcdfe6);
  border-radius: 8px;
  padding: 10px 12px;
}
.occupancy-columns h4 {
  margin: 0 0 8px;
  font-size: 14px;
}
.occupancy-columns small {
  display: block;
  color: var(--el-text-color-secondary, #909399);
}
.muted { color: var(--el-text-color-secondary, #909399); margin: 4px 0; }
</style>
