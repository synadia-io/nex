
<template>
    <div>
        <div class="sm:flex-auto">
            <h1 class="text-base font-semibold leading-6 text-gray-900">Dashboard</h1>
            <p class="mt-2 text-sm text-gray-700">Summary of all discovered namespaces</p>
        </div>
        <br />
    </div>
    <ul role="list" class="grid grid-cols-1 gap-x-6 gap-y-8 lg:grid-cols-3 xl:gap-x-8">
        <NamespaceCard v-for="card in cards" :card="card"/>        
    </ul>
</template>

<script setup lang="ts">
import { Ref, ref } from 'vue';
import NamespaceCard, { NamespaceSummaryCard } from './NamespaceCard.vue';

const cards : Ref<NamespaceSummaryCard[]> = ref([
    { namespace: "floogle", last_event_human: "3 hours", last_log_human: "2 hours", total_workloads: 7},
    { namespace: "dongle", last_event_human: "2 hours", last_log_human: "1 hour", total_workloads: 3}
])

</script>