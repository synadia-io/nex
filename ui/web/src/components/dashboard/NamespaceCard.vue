
<template>
    <li class="overflow-hidden rounded-xl border border-gray-200">
        <div class="flex items-center gap-x-4 border-b border-indigo-900/5 bg-indigo-50 p-6">
            <div class="text-sm font-medium leading-6 text-indigo-900">
                {{ card.namespace }}
            </div>            
        </div>
        <dl class="-my-3 divide-y divide-gray-100 px-6 py-4 text-sm leading-6">
            <div class="flex justify-between gap-x-4 py-3">
                <dt class="text-gray-500">Last Event</dt>
                <dd class="text-gray-700"><time datetime="2022-12-13">
                         {{ card.last_event_human }}
                    </time></dd>
            </div>
            <div class="flex justify-between gap-x-4 py-3">
                <dt class="text-gray-500">Last Log</dt>
                <dd class="text-gray-700"><time datetime="2022-12-13">
                        {{  card.last_log_human }}
                    </time></dd>
            </div>
            <div class="flex justify-between gap-x-4 py-3">
                <dt class="text-gray-500">Active Workloads</dt>
                <dd class="flex items-start gap-x-2">
                    <div class="font-medium text-gray-900">
                        {{ card.total_workloads }}
                    </div>
                </dd>
            </div>
        </dl>
    </li>
</template>

<script setup lang="ts">
defineProps<{
    card: NamespaceSummaryCard
}>()

export interface NamespaceSummaryCard {
    last_event_human: String
    last_log_human: String
    total_workloads: Number
    namespace: String
}
</script>