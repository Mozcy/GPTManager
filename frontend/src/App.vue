<script setup>
import AccountManager from './components/AccountManager.vue'
import UpstreamStatusBar from './components/UpstreamStatusBar.vue'
</script>

<template>
  <main class="app-page">
    <AccountManager />
    <UpstreamStatusBar />
  </main>
</template>

<style>
.app-page {
  width: 100%;
  min-height: 100vh;
  padding: 10px;
  box-sizing: border-box;
}
</style>
