<script setup>
import AccountManager from './components/AccountManager.vue'
import CreateProxy from './components/CreateProxy.vue'
import UpstreamStatusBar from './components/UpstreamStatusBar.vue'
</script>

<template>
  <main class="app-page">
    <CreateProxy />
    <AccountManager />
    <UpstreamStatusBar />
  </main>
</template>

<style>
.app-page {
  width: 100%;
  box-sizing: border-box;
}
</style>
