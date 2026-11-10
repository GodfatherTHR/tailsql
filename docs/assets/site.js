(function () {
  "use strict";

  var navToggle = document.querySelector("[data-nav-toggle]");
  var nav = document.querySelector("[data-nav]");

  if (navToggle && nav) {
    navToggle.addEventListener("click", function () {
      var open = nav.classList.toggle("is-open");
      navToggle.setAttribute("aria-expanded", open ? "true" : "false");
      navToggle.textContent = open ? "Close" : "Menu";
    });

    nav.querySelectorAll("a").forEach(function (link) {
      link.addEventListener("click", function () {
        nav.classList.remove("is-open");
        navToggle.setAttribute("aria-expanded", "false");
        navToggle.textContent = "Menu";
      });
    });
  }

  function copyText(text, button) {
    var done = function () {
      var original = button.getAttribute("data-label") || button.textContent;
      button.setAttribute("data-label", original);
      button.textContent = "Copied";
      window.setTimeout(function () {
        button.textContent = original;
      }, 1400);
    };

    if (navigator.clipboard && window.isSecureContext) {
      navigator.clipboard.writeText(text).then(done).catch(function () {
        fallbackCopy(text, done);
      });
      return;
    }
    fallbackCopy(text, done);
  }

  function fallbackCopy(text, done) {
    var area = document.createElement("textarea");
    area.value = text;
    area.setAttribute("readonly", "");
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.appendChild(area);
    area.select();
    try {
      document.execCommand("copy");
      done();
    } catch (error) {
      // Clipboard access is unavailable; leave the button unchanged.
    }
    document.body.removeChild(area);
  }

  document.querySelectorAll("[data-copy]").forEach(function (button) {
    button.addEventListener("click", function () {
      var block = button.closest(".code-block");
      var code = block ? block.querySelector("pre code") : null;
      if (code) {
        copyText(code.innerText, button);
      }
    });
  });

  var reveals = document.querySelectorAll(".reveal");
  if (reveals.length && "IntersectionObserver" in window) {
    var revealObserver = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (entry.isIntersecting) {
            entry.target.classList.add("is-visible");
            revealObserver.unobserve(entry.target);
          }
        });
      },
      { rootMargin: "0px 0px -8% 0px", threshold: 0.08 }
    );
    reveals.forEach(function (element) {
      revealObserver.observe(element);
    });
  } else {
    reveals.forEach(function (element) {
      element.classList.add("is-visible");
    });
  }

  var docsNavLinks = document.querySelectorAll(".docs-nav-group a[href^='#']");
  if (docsNavLinks.length && "IntersectionObserver" in window) {
    var sections = [];
    docsNavLinks.forEach(function (link) {
      var id = link.getAttribute("href");
      if (!id || id === "#") {
        return;
      }
      var section = document.querySelector(id);
      if (section) {
        sections.push({ link: link, section: section });
      }
    });

    if (sections.length) {
      var activeObserver = new IntersectionObserver(
        function (entries) {
          entries.forEach(function (entry) {
            if (!entry.isIntersecting) {
              return;
            }
            sections.forEach(function (item) {
              var active = item.section === entry.target;
              item.link.classList.toggle("active", active);
              if (active) {
                item.link.setAttribute("aria-current", "true");
              } else {
                item.link.removeAttribute("aria-current");
              }
            });
          });
        },
        { rootMargin: "-18% 0px -68% 0px", threshold: 0.01 }
      );
      sections.forEach(function (item) {
        activeObserver.observe(item.section);
      });
    }
  }

  document.querySelectorAll("[data-year]").forEach(function (element) {
    element.textContent = String(new Date().getFullYear());
  });
})();
