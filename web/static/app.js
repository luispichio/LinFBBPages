(function () {
  "use strict";

  const state = {
    language: localStorage.getItem("linfbb_language") || detectLanguage(),
    user: null,
    page: 1,
    totalPages: 1,
    view: "messages-view"
  };

  const $ = (selector) => document.querySelector(selector);

  function detectLanguage() {
    return navigator.language && navigator.language.toLowerCase().startsWith("en") ? "en" : "es";
  }

  function translate(key) {
    const dictionary = window.LIN_FBB_I18N[state.language] || window.LIN_FBB_I18N.es;
    return dictionary[key] || key;
  }

  function format(key, values) {
    return translate(key).replace(/\{(\w+)\}/g, (_, name) => values[name] == null ? "" : values[name]);
  }

  function applyTranslations() {
    document.documentElement.lang = state.language;
    document.querySelectorAll("[data-i18n]").forEach((element) => {
      element.textContent = translate(element.dataset.i18n);
    });
    document.querySelectorAll("[data-i18n-placeholder]").forEach((element) => {
      element.placeholder = translate(element.dataset.i18nPlaceholder);
    });
    $("#login-language").value = state.language;
    $("#app-language").value = state.language;
    if (state.user) {
      renderUser();
    }
  }

  function setLanguage(language) {
    state.language = language === "en" ? "en" : "es";
    localStorage.setItem("linfbb_language", state.language);
    applyTranslations();
    if (!$("#app-view").hidden && state.view === "messages-view") {
      loadMessages();
    }
  }

  async function request(path, options) {
    const response = await fetch(path, Object.assign({
      credentials: "same-origin",
      headers: { "Accept": "application/json" }
    }, options || {}));
    let data = null;
    try {
      data = await response.json();
    } catch (_) {
      data = {};
    }
    if (response.status === 401) {
      throw Object.assign(new Error(translate("errors.session")), { sessionExpired: true });
    }
    if (!response.ok) {
      throw new Error(data.error || translate("errors.generic"));
    }
    return data;
  }

  function showLogin() {
    $("#login-view").hidden = false;
    $("#app-view").hidden = true;
    state.user = null;
  }

  function showApp(user) {
    state.user = user;
    $("#login-view").hidden = true;
    $("#app-view").hidden = false;
    renderUser();
    setView("messages-view");
    loadMessages();
  }

  function renderUser() {
    const user = state.user;
    if (!user) {
      $("#current-user").textContent = "";
      return;
    }
    const fullName = [user.first_name, user.name].filter(Boolean).join(" ");
    $("#current-user").textContent = fullName ? `${user.callsign} · ${fullName}` : user.callsign;
  }

  function setView(view) {
    state.view = view;
    document.querySelectorAll(".view").forEach((element) => {
      element.hidden = element.id !== view;
    });
    document.querySelectorAll(".tab").forEach((button) => {
      button.classList.toggle("active", button.dataset.view === view);
    });
    hideBanner("#app-error");
    if (view === "messages-view") {
      loadMessages();
    } else if (view === "files-view") {
      loadFiles();
    }
  }

  function showBanner(selector, message) {
    const element = $(selector);
    element.textContent = message;
    element.hidden = false;
  }

  function hideBanner(selector) {
    $(selector).hidden = true;
    $(selector).textContent = "";
  }

  function handleError(error, selector) {
    if (error.sessionExpired) {
      showLogin();
      showBanner("#login-error", translate("errors.session"));
      return;
    }
    showBanner(selector || "#app-error", error.message || translate("errors.generic"));
  }

  function formatDate(seconds) {
    if (!seconds) return "—";
    return new Intl.DateTimeFormat(state.language, {
      dateStyle: "short",
      timeStyle: "short",
      timeZone: "UTC"
    }).format(new Date(Number(seconds) * 1000));
  }

  async function loadSession() {
    try {
      const data = await request("/api/me");
      showApp(data.user);
    } catch (error) {
      if (!error.sessionExpired) showLogin();
    }
  }

  async function login(event) {
    event.preventDefault();
    hideBanner("#login-error");
    const form = new FormData(event.currentTarget);
    try {
      const data = await request("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Accept": "application/json" },
        body: JSON.stringify({ callsign: form.get("callsign"), password: form.get("password") })
      });
      event.currentTarget.reset();
      showApp(data.user);
      showBanner("#app-notice", translate("notices.welcome"));
    } catch (error) {
      showBanner("#login-error", error.message || translate("errors.login"));
    }
  }

  async function logout() {
    try {
      await request("/api/logout", { method: "POST" });
    } catch (_) {
      // Even if the server is unavailable, remove the local application state.
    }
    showLogin();
  }

  async function loadMessages() {
    if ($("#messages-view").hidden) return;
    const form = new FormData($("#message-filters"));
    const params = new URLSearchParams({ page: String(state.page), page_size: "50" });
    if (form.get("q")) params.set("q", form.get("q"));
    if (form.get("type")) params.set("type", form.get("type"));
    try {
      const data = await request(`/api/messages?${params}`);
      state.totalPages = Math.max(1, data.total_pages || 1);
      renderMessages(data.messages || []);
      $("#page-label").textContent = format("messages.page", { page: state.page, pages: state.totalPages });
      $("#previous-page").disabled = state.page <= 1;
      $("#next-page").disabled = state.page >= state.totalPages;
    } catch (error) {
      handleError(error);
    }
  }

  function renderMessages(messages) {
    const body = $("#messages-body");
    body.replaceChildren();
    $("#messages-empty").hidden = messages.length !== 0;
    messages.forEach((message) => {
      const row = document.createElement("tr");
      [message.number, message.type, message.status, message.from || "—", message.to || "—"].forEach((value) => {
        const cell = document.createElement("td");
        cell.textContent = value;
        row.appendChild(cell);
      });
      const subject = document.createElement("td");
      const button = document.createElement("button");
      button.className = "table-link";
      button.type = "button";
      button.textContent = message.title || "(sin asunto)";
      button.addEventListener("click", () => loadMessage(message.number));
      subject.appendChild(button);
      row.appendChild(subject);
      const date = document.createElement("td");
      date.textContent = formatDate(message.date);
      row.appendChild(date);
      body.appendChild(row);
    });
  }

  async function loadMessage(number) {
    try {
      const data = await request(`/api/messages/${encodeURIComponent(number)}`);
      renderMessageDetail(data.message, data.body);
      setView("message-detail");
    } catch (error) {
      handleError(error);
    }
  }

  function renderMessageDetail(message, body) {
    $("#detail-title").textContent = `#${message.number} · ${message.title || "(sin asunto)"}`;
    const meta = $("#detail-meta");
    meta.replaceChildren();
    [["messages.detailFrom", message.from || "—"], ["messages.detailTo", message.to || "—"], ["messages.detailType", message.type], ["messages.detailStatus", message.status], ["messages.detailDate", formatDate(message.date)]].forEach(([key, value]) => {
      const term = document.createElement("dt");
      term.textContent = translate(key);
      const description = document.createElement("dd");
      description.textContent = value;
      meta.append(term, description);
    });
    const routing = $("#detail-routing");
    routing.replaceChildren();
    const routes = body && body.routing ? body.routing : [];
    if (routes.length) {
      const title = document.createElement("strong");
      title.textContent = `${translate("messages.routing")}:`;
      routing.appendChild(title);
      routes.forEach((route) => {
        const line = document.createElement("div");
        line.textContent = route;
        routing.appendChild(line);
      });
    } else {
      routing.textContent = translate("notices.noRouting");
    }
    $("#detail-body").textContent = body && body.body ? body.body : "";
  }

  async function submitCompose(event) {
    event.preventDefault();
    hideBanner("#compose-error");
    const form = new FormData(event.currentTarget);
    try {
      const data = await request("/api/messages", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Accept": "application/json" },
        body: JSON.stringify({
          type: form.get("type"),
          to: form.get("to"),
          route: form.get("route"),
          title: form.get("title"),
          body: form.get("body")
        })
      });
      event.currentTarget.reset();
      showBanner("#app-notice", data.message || translate("notices.sent"));
      setView("messages-view");
    } catch (error) {
      handleError(error, "#compose-error");
    }
  }

  async function loadFiles() {
    if ($("#files-view").hidden) return;
    try {
      const data = await request("/api/files");
      renderFiles(data.files || []);
    } catch (error) {
      handleError(error);
    }
  }

  function renderFiles(groups) {
    const grid = $("#files-grid");
    grid.replaceChildren();
    $("#files-empty").hidden = groups.length !== 0;
    groups.forEach((group) => {
      const card = document.createElement("article");
      card.className = "file-card";
      if (group.primary && group.primary.mime && group.primary.mime.startsWith("image/")) {
        const image = document.createElement("img");
        image.loading = "lazy";
        image.alt = group.primary.name;
        image.src = `/api/files/${encodeURIComponent(group.primary.name)}`;
        card.appendChild(image);
      } else {
        const icon = document.createElement("div");
        icon.className = "file-icon";
        icon.textContent = "7+";
        card.appendChild(icon);
      }
      const content = document.createElement("div");
      content.className = "file-card-content";
      const title = document.createElement("h3");
      title.textContent = group.primary ? group.primary.name : group.name;
      content.appendChild(title);
      if (group.primary) {
        const details = document.createElement("p");
        details.className = "muted small";
        details.textContent = `${formatBytes(group.primary.size)} · ${group.primary.mime}`;
        content.appendChild(details);
        const link = document.createElement("a");
        link.className = "table-link";
        link.href = `/api/files/${encodeURIComponent(group.primary.name)}`;
        link.target = "_blank";
        link.rel = "noopener";
        link.textContent = translate("files.download");
        content.appendChild(link);
      }
      if (group.auxiliary && group.auxiliary.length) {
        const aux = document.createElement("p");
        aux.className = "muted small";
        aux.textContent = `${translate("files.auxiliary")}: ${group.auxiliary.map((file) => file.name).join(", ")}`;
        content.appendChild(aux);
      }
      card.appendChild(content);
      grid.appendChild(card);
    });
  }

  function formatBytes(size) {
    if (size < 1024) return `${size} B`;
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
    return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  }

  function init() {
    applyTranslations();
    $("#login-form").addEventListener("submit", login);
    $("#logout-button").addEventListener("click", logout);
    $("#login-language").addEventListener("change", (event) => setLanguage(event.target.value));
    $("#app-language").addEventListener("change", (event) => setLanguage(event.target.value));
    document.querySelectorAll(".tab").forEach((button) => button.addEventListener("click", () => setView(button.dataset.view)));
    $("#message-filters").addEventListener("submit", (event) => {
      event.preventDefault();
      state.page = 1;
      loadMessages();
    });
    $("#refresh-messages").addEventListener("click", loadMessages);
    $("#previous-page").addEventListener("click", () => { if (state.page > 1) { state.page--; loadMessages(); } });
    $("#next-page").addEventListener("click", () => { if (state.page < state.totalPages) { state.page++; loadMessages(); } });
    $("#close-detail").addEventListener("click", () => setView("messages-view"));
    $("#compose-form").addEventListener("submit", submitCompose);
    $("#refresh-files").addEventListener("click", loadFiles);
    loadSession();
  }

  document.addEventListener("DOMContentLoaded", init);
}());
