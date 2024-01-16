<script setup lang="ts">
console.log("LOGS SETUP");

import { Ref, ref } from 'vue';
import LogLine, { LogLineData } from './LogLine.vue';

const logs: Ref<LogLineData[]> = ref([
    { text: "foobar", level: "debug", namespace: "default", workload_id: "echoservice", timestamp: "now" }
])

// TODO: update the ref of log lines when we get messages from the socket
const ws = new WebSocket("ws://" + document.location.host + "/ws");
ws.addEventListener("message", async (event) => {
    console.log("Message from server ", event.data);
    const raw = await event.data.text();
    const logObject = JSON.parse(raw);
    logs.value.unshift({
        text: logObject.text,
        level: logObject.level,
        namespace: logObject.namespace,
        workload_id: logObject.workload_id,
        timestamp: logObject.timestamp
    })
});
ws.addEventListener("open", (event) => {
  console.log("Connected to socket server", event);
});

</script>

<template>
    <div class="px-4 sm:px-6 lg:px-8">
  <div class="sm:flex sm:items-center">
    <div class="sm:flex-auto">
      <h1 class="text-base font-semibold leading-6 text-gray-900">Logs</h1>
      <p class="mt-2 text-sm text-gray-700">Aggregate log output from available workloads</p>
    </div>
  </div>
  <div class="mt-8 flow-root">
    <div class="-mx-4 -my-2 sm:-mx-6 lg:-mx-8">
      <div class="inline-block min-w-full py-2 align-middle">
        <table class="min-w-full border-separate border-spacing-0">
          <thead>
            <tr>
              <th
                scope="col"
                class="sticky top-0 z-10 border-b border-gray-300 bg-white bg-opacity-75 py-3.5 pl-4 pr-3 text-left text-sm font-semibold text-gray-900 backdrop-blur backdrop-filter sm:pl-6 lg:pl-8"
              >
                Level
              </th>
              <th
                scope="col"
                class="sticky top-0 z-10 border-b border-gray-300 bg-white bg-opacity-75 py-3.5 pl-4 pr-3 text-left text-sm font-semibold text-gray-900 backdrop-blur backdrop-filter sm:pl-6 lg:pl-8"
              >
                Namespace
              </th>

              <th
                scope="col"
                class="sticky top-0 z-10 hidden border-b border-gray-300 bg-white bg-opacity-75 px-3 py-3.5 text-left text-sm font-semibold text-gray-900 backdrop-blur backdrop-filter sm:table-cell"
              >
                Workload ID
              </th>
              <th
                scope="col"
                class="w-1/2 sticky top-0 z-10 hidden border-b border-gray-300 bg-white bg-opacity-75 px-3 py-3.5 text-left text-sm font-semibold text-gray-900 backdrop-blur backdrop-filter lg:table-cell"
              >
                Text
              </th>
              <th
                scope="col"
                class="text-right sticky top-0 z-10 border-b border-gray-300 bg-white bg-opacity-75 px-3 py-3.5 text-left text-sm font-semibold text-gray-900 backdrop-blur backdrop-filter"
              >
                Time
              </th>
            </tr>
          </thead>
          <tbody>
            <LogLine v-for="log in logs" :logline="log"/> 
          </tbody>
        </table>
      </div>
    </div>
  </div>
</div>
      
</template>