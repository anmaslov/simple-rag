<template>
  <div class="shell">
    <aside class="nav">
      <h1>Confluence RAG</h1>
      <RouterLink to="/chat">Chat</RouterLink>
      <RouterLink to="/search">Search</RouterLink>
      <RouterLink to="/pages">Pages</RouterLink>
      <RouterLink to="/sync">Sync</RouterLink>
      <RouterLink to="/settings">Settings</RouterLink>
    </aside>
    <main class="main">
      <RouterView />
    </main>
  </div>
</template>
