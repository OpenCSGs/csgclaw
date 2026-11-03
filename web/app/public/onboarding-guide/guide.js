(function () {
  let initialized = false;
  if (window.parent !== window && new URLSearchParams(location.search).get("embedded") === "1") {
    const exitButton = document.getElementById("exitGuide");
    if (exitButton) exitButton.style.visibility = "hidden";
  }
  const { t } = window.CSGClawGuideI18n;
  const slides = Array.from(document.querySelectorAll(".slide"));
  const dotsWrap = document.getElementById("dots");
  const prevBtn = document.getElementById("prevBtn");
  const nextBtn = document.getElementById("nextBtn");
  const counterNow = document.getElementById("counterNow");
  const counterTotal = document.getElementById("counterTotal");
  const progress = document.getElementById("progress");
  const dockChapter = document.getElementById("dockChapter");
  const chapterBtns = Array.from(document.querySelectorAll(".chapters button"));
  const stepSlides = slides.filter((s) => s.dataset.step);
  const total = stepSlides.length;
  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  let current = 0;
  let scrollTarget = null;
  let scrollSettleTimer = null;

  function finishNavigation() {
    scrollTarget = null;
    clearTimeout(scrollSettleTimer);
    onScroll();
  }

  function waitForScrollToSettle() {
    clearTimeout(scrollSettleTimer);
    // Fallback for browsers without scrollend, including interrupted scrolls.
    scrollSettleTimer = setTimeout(finishNavigation, 180);
  }

  function interruptNavigation() {
    if (scrollTarget === null) return;
    finishNavigation();
    window.scrollTo({ top: window.scrollY, behavior: "instant" });
  }

  // 圆点：封面 + 10 个步骤
  const dotSlides = [slides[0], ...stepSlides];
  dotSlides.forEach((slide, i) => {
    const b = document.createElement("button");
    b.type = "button";
    b.setAttribute("role", "tab");
    b.setAttribute("aria-label", i === 0 ? t("nav.cover") : t("nav.step").replace("{step}", String(i)));
    b.addEventListener("click", () => go(i));
    dotsWrap.appendChild(b);
  });
  const dots = Array.from(dotsWrap.children);

  function go(i, opts) {
    i = Math.max(0, Math.min(dotSlides.length - 1, i));
    const behavior = reduce || (opts && opts.instant) ? "instant" : "smooth";
    const el = dotSlides[i];
    scrollTarget = i;
    setActive(i);
    el.scrollIntoView({ behavior, block: "start" });
    waitForScrollToSettle();
    try {
      if (history.replaceState)
        history.replaceState(null, "", i === 0 ? location.pathname + location.search : "#" + el.id);
    } catch {
      /* File previews may not allow history updates. */
    }
  }

  function setActive(i) {
    current = i;
    const slide = dotSlides[i];
    const step = slide.dataset.step ? Number(slide.dataset.step) : 0;
    slides.forEach((s) => s.classList.toggle("is-active", s === slide));

    dots.forEach((d, di) => {
      d.setAttribute("aria-current", di === i ? "true" : "false");
      d.tabIndex = di === i ? 0 : -1;
    });

    if (step === 0) {
      counterNow.textContent = t("nav.start");
      counterTotal.textContent = "/ " + total;
      dockChapter.textContent = t("nav.coverDescription");
      progress.style.width = "0%";
      chapterBtns.forEach((b) => b.removeAttribute("aria-current"));
    } else {
      counterNow.textContent = String(step).padStart(2, "0");
      counterTotal.textContent = "/ " + total;
      const basic = step <= 5;
      dockChapter.textContent =
        t(basic ? "nav.gettingStarted" : "nav.nextLevel") + " · " + (basic ? step : step - 5) + " / 5";
      progress.style.width = (step / total) * 100 + "%";
      chapterBtns.forEach((b) => {
        if (b.dataset.goto === (basic ? "1" : "6")) b.setAttribute("aria-current", "true");
        else b.removeAttribute("aria-current");
      });
    }

    prevBtn.disabled = i === 0;
    nextBtn.disabled = i === dotSlides.length - 1;
  }

  prevBtn.addEventListener("click", () => go(current - 1));
  nextBtn.addEventListener("click", () => go(current + 1));
  document.getElementById("startGuide")?.addEventListener("click", () => go(1));
  document
    .querySelectorAll("[data-next]:not(#startGuide)")
    .forEach((b) => b.addEventListener("click", () => go(current + 1)));
  document.querySelectorAll("[data-goto]").forEach((b) => {
    b.addEventListener("click", () => {
      const n = Number(b.dataset.goto);
      go(n === 0 ? 0 : dotSlides.findIndex((s) => s.dataset.step === String(n)));
    });
  });

  // 退出导览：嵌入 App 时由宿主关闭；独立打开时尽量返回上一页
  let toastTimer = null;
  function showToast(msg) {
    const t = document.getElementById("toast");
    if (!t) return;
    t.textContent = msg;
    t.classList.add("show");
    clearTimeout(toastTimer);
    toastTimer = setTimeout(() => t.classList.remove("show"), 2400);
  }
  function exitGuide() {
    const evt = new CustomEvent("csgclaw:guide-exit", { cancelable: true });
    if (!window.dispatchEvent(evt)) return; // 宿主已接管
    if (window.parent && window.parent !== window) {
      try {
        window.parent.postMessage({ type: "csgclaw:guide-exit" }, window.location.origin);
      } catch {
        /* The host close button remains available. */
      }
      return;
    }
    if (window.opener) {
      try {
        window.close();
        return;
      } catch {
        /* Fall back to browser history. */
      }
    }
    if (history.length > 1) {
      history.back();
      return;
    }
    showToast(t("nav.canClose"));
  }
  document.getElementById("exitGuide")?.addEventListener("click", exitGuide);
  document.querySelectorAll("[data-exit]").forEach((b) => b.addEventListener("click", exitGuide));

  window.addEventListener("message", (event) => {
    if (event.source !== window.parent || event.origin !== window.location.origin) return;
    if (window.parent !== window && event.data?.type === "csgclaw:guide-layout") {
      const { controlsWidth, exitWidth } = event.data;
      if (Number.isFinite(controlsWidth) && controlsWidth >= 0 && controlsWidth <= 300) {
        document.documentElement.style.setProperty("--guide-controls-width", `${controlsWidth}px`);
      }
      if (Number.isFinite(exitWidth) && exitWidth > 0 && exitWidth <= 400) {
        const exitButton = document.getElementById("exitGuide");
        if (exitButton?.style.visibility === "hidden") exitButton.style.width = `${exitWidth}px`;
      }
    }
    if (event.data?.type === "csgclaw:guide-focus") {
      document.getElementById("startGuide")?.focus();
      reportReady();
    }
  });

  // 键盘
  document.addEventListener("keydown", (e) => {
    const tag = (e.target.tagName || "").toLowerCase();
    if (e.key === "Tab") {
      const focusable = Array.from(
        document.querySelectorAll('a[href], button:not([disabled]), [tabindex]:not([tabindex="-1"])'),
      ).filter((element) => element instanceof HTMLElement && element.offsetParent !== null);
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (e.shiftKey && (document.activeElement === first || document.activeElement === document.body)) {
        e.preventDefault();
        last?.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first?.focus();
      }
      return;
    }
    if (tag === "input" || tag === "textarea" || e.target.isContentEditable) return;
    if (["ArrowRight", "ArrowDown", "PageDown", " "].includes(e.key)) {
      e.preventDefault();
      go(current + 1);
    } else if (["ArrowLeft", "ArrowUp", "PageUp"].includes(e.key)) {
      e.preventDefault();
      go(current - 1);
    } else if (e.key === "Escape") {
      e.preventDefault();
      exitGuide();
    } else if (e.key === "Home") {
      e.preventDefault();
      go(0);
    } else if (e.key === "End") {
      e.preventDefault();
      go(dotSlides.length - 1);
    }
  });

  // 滚动观察：取距离视口中线最近的幻灯片
  let ticking = false;
  function onScroll() {
    if (scrollTarget !== null) {
      waitForScrollToSettle();
      return;
    }
    if (ticking) return;
    ticking = true;
    requestAnimationFrame(() => {
      ticking = false;
      // A frame queued before navigation must not restore an intermediate page.
      if (scrollTarget !== null) return;
      const mid = window.scrollY + window.innerHeight * 0.45;
      let best = 0,
        bestDist = Infinity;
      dotSlides.forEach((s, i) => {
        const rect = s.getBoundingClientRect();
        const top = window.scrollY + rect.top;
        const center = top + Math.min(rect.height, window.innerHeight) * 0.5;
        const d = Math.abs(center - mid);
        if (d < bestDist) {
          bestDist = d;
          best = i;
        }
      });
      if (best !== current) setActive(best);
    });
  }
  window.addEventListener("scroll", onScroll, { passive: true });
  window.addEventListener("resize", onScroll);
  window.addEventListener("scrollend", () => {
    if (scrollTarget !== null && Math.abs(dotSlides[scrollTarget].getBoundingClientRect().top) <= 2) {
      finishNavigation();
    }
  });
  window.addEventListener("wheel", interruptNavigation, { passive: true });

  // 首屏定位：等布局稳定后再跳转
  if ("scrollRestoration" in history) history.scrollRestoration = "manual";
  window.scrollTo(0, 0);
  setActive(0);
  const hash = decodeURIComponent(location.hash.replace("#", ""));
  const target = dotSlides.findIndex((s) => s.id === hash);
  if (target > 0) {
    requestAnimationFrame(() =>
      requestAnimationFrame(() => {
        go(target, { instant: true });
      }),
    );
  }

  // 触屏左右滑动
  let touchX = null,
    touchY = null;
  window.addEventListener(
    "touchstart",
    (e) => {
      interruptNavigation();
      touchX = e.touches[0].clientX;
      touchY = e.touches[0].clientY;
    },
    { passive: true },
  );
  window.addEventListener(
    "touchend",
    (e) => {
      if (touchX == null) return;
      const dx = e.changedTouches[0].clientX - touchX;
      const dy = e.changedTouches[0].clientY - touchY;
      if (Math.abs(dx) > 70 && Math.abs(dx) > Math.abs(dy) * 1.6) {
        go(current + (dx < 0 ? 1 : -1));
      }
      touchX = touchY = null;
    },
    { passive: true },
  );
  function reportReady() {
    if (!initialized || window.parent === window) return;
    // A load event can also mean an error page. Confirm script initialization
    // and allow the first view to render before the host can acknowledge it.
    requestAnimationFrame(() =>
      requestAnimationFrame(() => {
        window.parent.postMessage({ type: "csgclaw:guide-ready" }, window.location.origin);
      }),
    );
  }
  initialized = true;
  reportReady();
})();
