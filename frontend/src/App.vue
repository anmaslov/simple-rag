<template>
  <div class="shell">
    <aside class="nav">
      <h1>Knowledge RAG</h1>
      <RouterLink to="/chat">Chat</RouterLink>
      <RouterLink to="/search">Search</RouterLink>
      <RouterLink to="/pages">Documents</RouterLink>
      <RouterLink to="/sources">Sources</RouterLink>
      <RouterLink to="/settings">Settings</RouterLink>
    </aside>
    <main class="main">
      <RouterView />
    </main>
  </div>
</template>
