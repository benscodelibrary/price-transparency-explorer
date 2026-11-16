<script setup lang="ts">
import StatusBanner from './components/StatusBanner.vue'
import SourceInfo from './components/SourceInfo.vue'
</script>

<template>
  <header class="app-header">
    <div class="container header-inner">
      <a class="brand" href="/" aria-label="Price Transparency Explorer home">
        <span class="logo" aria-hidden="true">$</span>
        <span>
          <strong>Price Transparency Explorer</strong>
          <span class="tagline">Negotiated in-network rates from CMS Transparency in Coverage files</span>
        </span>
      </a>
    </div>
  </header>

  <StatusBanner />

  <main class="container main">
    <RouterView />
  </main>

  <footer class="container footer">
    <SourceInfo />
  </footer>
</template>

<style scoped>
.app-header {
  background: var(--surface);
  border-bottom: 1px solid var(--border);
}
.header-inner {
  display: flex;
  align-items: center;
  min-height: 64px;
}
.brand {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  color: var(--text);
  text-decoration: none;
}
.brand strong {
  display: block;
  font-size: 16px;
}
.tagline {
  display: block;
  font-size: 13px;
  color: var(--text-3);
}
.logo {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  border-radius: 9px;
  background: var(--primary);
  color: #fff;
  font-weight: 800;
  font-size: 18px;
}
.main {
  padding-top: var(--space-5);
  padding-bottom: var(--space-6);
}
.footer {
  padding-bottom: var(--space-6);
}
@media (max-width: 640px) {
  .tagline {
    display: none;
  }
}
</style>
