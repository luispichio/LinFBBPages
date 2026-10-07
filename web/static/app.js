(function () {
  "use strict";

  const state = {
    language: localStorage.getItem("linfbb_language") || detectLanguage(),
    user: null,
    page: 1,
    totalPages: 1,
    view: "messages-view",
    files: {
      groups: [],
      sort: ["date", "name", "type"].includes(localStorage.getItem("linfbb_files_sort")) ? localStorage.getItem("linfbb_files_sort") : "date",
      mode: ["cards", "list"].includes(localStorage.getItem("linfbb_files_mode")) ? localStorage.getItem("linfbb_files_mode") : "cards",
      query: ""
    },
    lightboxTrigger: null
  };
  let messageLoadToken = 0;

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

  function routeFromHash() {
    const hash = window.location.hash.slice(1);
    if (hash === "compose") return { view: "compose-view" };
    if (hash === "files") return { view: "files-view" };
    const messageMatch = /^msg-(\d+)$/.exec(hash);
    if (messageMatch) return { view: "message-detail", number: messageMatch[1] };
    return { view: "messages-view" };
  }

  function hashForView(view, messageNumber) {
    if (view === "compose-view") return "#compose";
    if (view === "files-view") return "#files";
    if (view === "message-detail" && /^\d+$/.test(String(messageNumber))) {
      return `#msg-${messageNumber}`;
    }
    return "#messages";
  }

  function updateHistory(view, messageNumber, historyMode) {
    if (historyMode === "none") return;
    const hash = hashForView(view, messageNumber);
    const routeState = { view, messageNumber: messageNumber == null ? null : String(messageNumber) };
    if (window.location.hash === hash || historyMode === "replace") {
      window.history.replaceState(routeState, "", hash);
      return;
    }
    window.history.pushState(routeState, "", hash);
  }

  function clearRouteHistory() {
    window.history.replaceState(null, "", `${window.location.pathname}${window.location.search}`);
  }

  function restoreRoute() {
    const route = routeFromHash();
    if (route.view === "message-detail") {
      loadMessage(route.number, { historyMode: "none" });
      return;
    }
    setView(route.view, { historyMode: "replace" });
  }

  function applyTranslations() {
    document.documentElement.lang = state.language;
    document.querySelectorAll("[data-i18n]").forEach((element) => {
      element.textContent = translate(element.dataset.i18n);
    });
    document.querySelectorAll("[data-i18n-placeholder]").forEach((element) => {
      element.placeholder = translate(element.dataset.i18nPlaceholder);
    });
    document.querySelectorAll("[data-i18n-aria-label]").forEach((element) => {
      element.setAttribute("aria-label", translate(element.dataset.i18nAriaLabel));
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
    } else if (!$("#app-view").hidden && state.view === "files-view") {
      renderFiles();
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
    if (response.status === 401 && path !== "/api/login") {
      throw Object.assign(new Error(translate("errors.session")), { sessionExpired: true, status: 401 });
    }
    if (!response.ok) {
      throw Object.assign(new Error(data.error || translate("errors.generic")), { status: response.status });
    }
    return data;
  }

  function showLogin() {
    messageLoadToken++;
    clearRouteHistory();
    $("#login-view").hidden = false;
    $("#app-view").hidden = true;
    state.user = null;
  }

  function showApp(user) {
    state.user = user;
    $("#login-view").hidden = true;
    $("#app-view").hidden = false;
    renderUser();
    restoreRoute();
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

  function setView(view, options) {
    const historyMode = options && options.historyMode ? options.historyMode : "push";
    const messageNumber = options && options.messageNumber;
    if (view !== "files-view") {
      closeFileLightbox();
    }
    if (view !== "message-detail") {
      messageLoadToken++;
    }
    state.view = view;
    updateHistory(view, messageNumber, historyMode);
    document.querySelectorAll(".view").forEach((element) => {
      element.hidden = element.id !== view;
    });
    document.querySelectorAll(".tab").forEach((button) => {
      button.classList.toggle("active", button.dataset.view === view);
      if (button.dataset.view === view) {
        button.setAttribute("aria-current", "page");
      } else {
        button.removeAttribute("aria-current");
      }
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
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
    try {
      const data = await request("/api/login", {
        method: "POST",
        headers: { "Content-Type": "application/json", "Accept": "application/json" },
        body: JSON.stringify({ callsign: form.get("callsign"), password: form.get("password") })
      });
      formElement.reset();
      showApp(data.user);
      showBanner("#app-notice", translate("notices.welcome"));
    } catch (error) {
      const message = error.status === 401 ? translate("errors.login") : (error.message || translate("errors.generic"));
      showBanner("#login-error", message);
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
    messageLoadToken++;
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

  async function loadMessage(number, options) {
    const historyMode = options && options.historyMode ? options.historyMode : "push";
    const requestToken = ++messageLoadToken;
    try {
      const data = await request(`/api/messages/${encodeURIComponent(number)}`);
      if (requestToken !== messageLoadToken) return;
      renderMessageDetail(data.message, data.body);
      setView("message-detail", { historyMode, messageNumber: number });
    } catch (error) {
      if (requestToken !== messageLoadToken) return;
      handleError(error);
      if (state.user && routeFromHash().view === "message-detail") {
        setView("messages-view", { historyMode: "replace" });
      }
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
    const formElement = event.currentTarget;
    const form = new FormData(formElement);
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
      formElement.reset();
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
      state.files.groups = data.files || [];
      renderFiles();
    } catch (error) {
      handleError(error);
    }
  }

  function setFileSort(sort) {
    state.files.sort = ["date", "name", "type"].includes(sort) ? sort : "date";
    localStorage.setItem("linfbb_files_sort", state.files.sort);
    renderFiles();
  }

  function setFileMode(mode) {
    state.files.mode = mode === "list" ? "list" : "cards";
    localStorage.setItem("linfbb_files_mode", state.files.mode);
    renderFiles();
  }

  function fileGroupFiles(group) {
    return [group.primary].concat(group.auxiliary || []).filter(Boolean);
  }

  function fileGroupName(group) {
    return group.primary ? group.primary.name : group.name;
  }

  function fileGroupRepresentative(group) {
    if (group.primary) return group.primary;
    return fileGroupFiles(group).reduce((latest, file) => {
      if (!latest) return file;
      return (Date.parse(file.modified) || 0) > (Date.parse(latest.modified) || 0) ? file : latest;
    }, null);
  }

  function fileGroupDate(group) {
    const representative = fileGroupRepresentative(group);
    return representative ? representative.modified : "";
  }

  function fileGroupType(group) {
    const representative = group.primary || fileGroupRepresentative(group);
    if (representative && representative.mime) return representative.mime.toLowerCase();
    const name = representative ? representative.name : group.name;
    const extension = name.lastIndexOf(".");
    return extension >= 0 ? name.slice(extension + 1).toLowerCase() : "";
  }

  function fileType(file) {
    if (!file) return "—";
    if (file.mime) return file.mime;
    const extension = file.name ? file.name.lastIndexOf(".") : -1;
    return extension >= 0 ? file.name.slice(extension + 1).toLowerCase() : "—";
  }

  function fileSize(file) {
    return file && Number.isFinite(Number(file.size)) ? formatBytes(Number(file.size)) : "—";
  }

  function fileGroupSearchText(group) {
    return [group.name].concat(fileGroupFiles(group).reduce((values, file) => values.concat(file.name, file.mime), [])).join(" ").toLowerCase();
  }

  function sortedFileGroups(groups) {
    const collator = new Intl.Collator(state.language, { numeric: true, sensitivity: "base" });
    return groups.slice().sort((left, right) => {
      if (state.files.sort === "date") {
        const dateDifference = (Date.parse(fileGroupDate(right)) || 0) - (Date.parse(fileGroupDate(left)) || 0);
        if (dateDifference !== 0) return dateDifference;
      } else if (state.files.sort === "type") {
        const typeDifference = collator.compare(fileGroupType(left), fileGroupType(right));
        if (typeDifference !== 0) return typeDifference;
      }
      return collator.compare(fileGroupName(left), fileGroupName(right));
    });
  }

  function formatFileDate(value) {
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return "—";
    return new Intl.DateTimeFormat(state.language, {
      dateStyle: "short",
      timeStyle: "short",
      timeZone: "UTC"
    }).format(date);
  }

  function syncFileControls() {
    const sort = $("#files-sort");
    const search = $("#files-search");
    const grid = $("#files-grid");
    if (sort) sort.value = state.files.sort;
    if (search && search.value !== state.files.query) search.value = state.files.query;
    if (grid) grid.dataset.mode = state.files.mode;
    document.querySelectorAll("#files-view-mode button").forEach((button) => {
      const active = button.dataset.mode === state.files.mode;
      button.classList.toggle("active", active);
      button.setAttribute("aria-pressed", String(active));
    });
  }

  function renderFiles() {
    const grid = $("#files-grid");
    const query = state.files.query.trim().toLowerCase();
    const groups = sortedFileGroups(state.files.groups.filter((group) => !query || fileGroupSearchText(group).includes(query)));
    grid.replaceChildren();
    syncFileControls();
    const empty = $("#files-empty");
    empty.hidden = groups.length !== 0;
    empty.textContent = query ? translate("files.noResults") : translate("files.empty");
    if (state.files.mode === "list" && groups.length) {
      renderFileDirectory(groups);
      return;
    }
    groups.forEach((group) => {
      const card = document.createElement("article");
      card.className = "file-card";
      if (group.primary && group.primary.mime && group.primary.mime.startsWith("image/")) {
        const preview = document.createElement("button");
        preview.className = "file-preview";
        preview.type = "button";
        preview.setAttribute("aria-label", format("files.previewLabel", { name: group.primary.name }));
        const image = document.createElement("img");
        image.loading = "lazy";
        image.alt = group.primary.name;
        image.src = `/api/files/${encodeURIComponent(group.primary.name)}`;
        preview.appendChild(image);
        preview.addEventListener("click", () => openFileLightbox(group.primary, preview));
        card.appendChild(preview);
      } else {
        const icon = document.createElement("div");
        icon.className = "file-icon";
        icon.textContent = group.primary ? "7+" : "META";
        card.appendChild(icon);
      }
      const content = document.createElement("div");
      content.className = "file-card-content";
      const title = document.createElement("h3");
      title.textContent = fileGroupName(group);
      content.appendChild(title);
      const representative = fileGroupRepresentative(group);
      if (representative) {
        const details = document.createElement("p");
        details.className = "muted small file-details";
        details.textContent = `${format("files.modified", { date: formatFileDate(representative.modified) })} · ${fileSize(representative)} · ${fileType(representative)}`;
        content.appendChild(details);
      }
      if (group.primary) {
        const actions = document.createElement("div");
        actions.className = "file-actions";
        const download = document.createElement("a");
        download.className = "secondary file-download";
        download.href = `/api/files/${encodeURIComponent(group.primary.name)}`;
        download.download = group.primary.name;
        download.textContent = translate("files.download");
        actions.appendChild(download);
        content.appendChild(actions);
      }
      if (group.auxiliary && group.auxiliary.length) {
        const aux = document.createElement("p");
        aux.className = "muted small file-auxiliary";
        aux.textContent = `${translate("files.auxiliary")}: ${group.auxiliary.map((file) => file.name).join(", ")}`;
        content.appendChild(aux);
      }
      card.appendChild(content);
      grid.appendChild(card);
    });
  }

  function renderFileDirectory(groups) {
    const wrapper = document.createElement("div");
    wrapper.className = "table-wrap files-directory";
    const table = document.createElement("table");
    const caption = document.createElement("caption");
    caption.className = "sr-only";
    caption.textContent = translate("files.tableCaption");
    table.appendChild(caption);

    const head = document.createElement("thead");
    const headerRow = document.createElement("tr");
    ["files.name", "files.type", "files.size", "files.date", "files.actions"].forEach((key) => {
      const cell = document.createElement("th");
      cell.scope = "col";
      cell.textContent = translate(key);
      headerRow.appendChild(cell);
    });
    head.appendChild(headerRow);
    table.appendChild(head);

    const body = document.createElement("tbody");
    groups.forEach((group) => {
      const row = document.createElement("tr");
      const representative = fileGroupRepresentative(group);

      const nameCell = document.createElement("td");
      nameCell.className = "file-directory-name";
      const nameLine = document.createElement("div");
      nameLine.className = "file-name-line";
      const marker = document.createElement("span");
      marker.className = "file-kind";
      marker.textContent = group.primary ? "7+" : "META";
      const name = document.createElement("span");
      name.className = "file-name";
      name.textContent = fileGroupName(group);
      nameLine.append(marker, name);
      nameCell.appendChild(nameLine);
      if (group.auxiliary && group.auxiliary.length) {
        const auxiliary = document.createElement("div");
        auxiliary.className = "muted small file-auxiliary";
        auxiliary.textContent = `${translate("files.auxiliary")}: ${group.auxiliary.map((file) => file.name).join(", ")}`;
        nameCell.appendChild(auxiliary);
      }
      row.appendChild(nameCell);

      const type = document.createElement("td");
      type.textContent = fileType(representative);
      row.appendChild(type);

      const size = document.createElement("td");
      size.textContent = fileSize(representative);
      row.appendChild(size);

      const date = document.createElement("td");
      date.textContent = representative ? formatFileDate(representative.modified) : "—";
      row.appendChild(date);

      const actions = document.createElement("td");
      if (group.primary) {
        const download = document.createElement("a");
        download.className = "secondary file-download";
        download.href = `/api/files/${encodeURIComponent(group.primary.name)}`;
        download.download = group.primary.name;
        download.textContent = translate("files.download");
        actions.appendChild(download);
      } else {
        actions.textContent = "—";
      }
      row.appendChild(actions);
      body.appendChild(row);
    });
    table.appendChild(body);
    wrapper.appendChild(table);
    $("#files-grid").appendChild(wrapper);
  }

  function openFileLightbox(file, trigger) {
    const lightbox = $("#file-lightbox");
    const image = $("#file-lightbox-image");
    const download = $("#file-lightbox-download");
    state.lightboxTrigger = trigger;
    $("#file-lightbox-title").textContent = file.name;
    image.src = `/api/files/${encodeURIComponent(file.name)}`;
    image.alt = file.name;
    download.href = `/api/files/${encodeURIComponent(file.name)}`;
    download.download = file.name;
    lightbox.hidden = false;
    document.body.classList.add("modal-open");
    $("#file-lightbox-close").focus();
  }

  function closeFileLightbox() {
    const lightbox = $("#file-lightbox");
    if (!lightbox || lightbox.hidden) return;
    lightbox.hidden = true;
    $("#file-lightbox-image").removeAttribute("src");
    $("#file-lightbox-download").removeAttribute("href");
    $("#file-lightbox-download").removeAttribute("download");
    document.body.classList.remove("modal-open");
    const trigger = state.lightboxTrigger;
    state.lightboxTrigger = null;
    if (trigger && document.contains(trigger)) trigger.focus();
  }

  function formatBytes(size) {
    if (size < 1024) return `${size} B`;
    if (size < 1024 * 1024) return `${(size / 1024).toFixed(1)} KB`;
    return `${(size / (1024 * 1024)).toFixed(1)} MB`;
  }

  function handleHistoryNavigation() {
    if (!state.user) {
      clearRouteHistory();
      return;
    }
    restoreRoute();
  }

  function init() {
    applyTranslations();
    $("#login-form").addEventListener("submit", login);
    $("#logout-button").addEventListener("click", logout);
    $("#login-language").addEventListener("change", (event) => setLanguage(event.target.value));
    $("#app-language").addEventListener("change", (event) => setLanguage(event.target.value));
    document.querySelectorAll(".tab").forEach((button) => button.addEventListener("click", () => setView(button.dataset.view)));
    window.addEventListener("popstate", handleHistoryNavigation);
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
    $("#files-search").addEventListener("input", (event) => {
      state.files.query = event.target.value;
      renderFiles();
    });
    $("#files-sort").addEventListener("change", (event) => setFileSort(event.target.value));
    document.querySelectorAll("#files-view-mode button").forEach((button) => {
      button.addEventListener("click", () => setFileMode(button.dataset.mode));
    });
    $("#file-lightbox-close").addEventListener("click", closeFileLightbox);
    $("#file-lightbox-backdrop").addEventListener("click", closeFileLightbox);
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape") closeFileLightbox();
    });
    loadSession();
  }

  document.addEventListener("DOMContentLoaded", init);
}());
