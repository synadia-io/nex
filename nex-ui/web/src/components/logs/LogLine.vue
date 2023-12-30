<template>
    <tr class="odd:bg-indigo-50">
      <td class="whitespace-nowrap border-b border-gray-200 py-4 pl-4 pr-3 text-sm font-medium text-gray-900 sm:pl-6 lg:pl-8">
        {{ logline.level }}
      </td>
      <td class="whitespace-nowrap border-b border-gray-200 py-4 pl-4 pr-3 text-sm font-medium text-gray-900 sm:pl-6 lg:pl-8">
        {{ logline.namespace }}
      </td>
      <td class="whitespace-nowrap border-b border-gray-200 hidden px-3 py-4 text-sm text-gray-500 sm:table-cell">
        {{ logline.workload_id }}
      </td>
      <td class="w-1/2 whitespace-nowrap border-b border-gray-200 hidden px-3 py-4 text-sm text-gray-500 lg:table-cell">
        {{ logline.text }}
      </td>
      <td class="text-right whitespace-nowrap border-b border-gray-200 px-3 py-4 text-sm text-gray-500">
        {{ logline.timestamp }}
      </td>
    </tr>
</template>


<script setup lang="ts">
defineProps<{
    logline: LogLineData
}>()

export interface LogLineData {
    text: string
    level: string
    namespace: string
    workload_id: string
    timestamp: string
}
</script>