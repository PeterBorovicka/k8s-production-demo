const message = document.querySelector("#message");
const environment = document.querySelector("#environment");
const version = document.querySelector("#version");
const reload = document.querySelector("#reload");

async function loadMessage() {
  reload.disabled = true;
  message.textContent = "Načítavam odpoveď backendu…";
  try {
    const response = await fetch("/api/message", {
      headers: { Accept: "application/json" },
      cache: "no-store",
    });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const data = await response.json();
    message.textContent = data.message;
    environment.textContent = data.environment;
    version.textContent = data.version;
  } catch (error) {
    message.textContent = `Backend nie je dostupný (${error.message}).`;
    environment.textContent = "—";
    version.textContent = "—";
  } finally {
    reload.disabled = false;
  }
}

reload.addEventListener("click", loadMessage);
loadMessage();

