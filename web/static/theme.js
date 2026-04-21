(function() {
    function applyIcons() {
        var isDark = document.documentElement.classList.contains('pf-v5-theme-dark');
        var sun = document.getElementById('theme-icon-sun');
        var moon = document.getElementById('theme-icon-moon');
        if (sun) sun.style.display = isDark ? 'inline' : 'none';
        if (moon) moon.style.display = isDark ? 'none' : 'inline';
    }
    window.toggleTheme = function() {
        var isDark = document.documentElement.classList.toggle('pf-v5-theme-dark');
        localStorage.setItem('theme', isDark ? 'dark' : 'light');
        applyIcons();
    };
    applyIcons();
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', function(e) {
        if (!localStorage.getItem('theme')) {
            document.documentElement.classList.toggle('pf-v5-theme-dark', e.matches);
            applyIcons();
        }
    });
    document.addEventListener('click', function(e) {
        var dd = document.getElementById('user-dropdown');
        if (dd && !dd.contains(e.target)) {
            dd.classList.remove('pf-m-expanded');
            var btn = dd.querySelector('.pf-v5-c-dropdown__toggle');
            if (btn) btn.setAttribute('aria-expanded', 'false');
        }
    });

    var _pfConfirmResolve = null;

    function ensureConfirmModal() {
        if (document.getElementById('pf-confirm-backdrop')) return;
        var html = '<div class="pf-v5-c-backdrop" id="pf-confirm-backdrop" style="display:none;">' +
            '<div class="pf-v5-l-bullseye">' +
            '<div class="pf-v5-c-modal-box pf-m-sm" role="dialog" aria-modal="true" aria-labelledby="pf-confirm-title">' +
            '<div class="pf-v5-c-modal-box__close">' +
            '<button class="pf-v5-c-button pf-m-plain" aria-label="Close" onclick="pfConfirmClose(false)">' +
            '<svg fill="currentColor" height="1em" width="1em" viewBox="0 0 352 512"><path d="M242.72 256l100.07-100.07c12.28-12.28 12.28-32.19 0-44.48l-22.24-22.24c-12.28-12.28-32.19-12.28-44.48 0L176 189.28 75.93 89.21c-12.28-12.28-32.19-12.28-44.48 0L9.21 111.45c-12.28 12.29-12.28 32.2 0 44.48L109.28 256 9.21 356.07c-12.28 12.28-12.28 32.19 0 44.48l22.24 22.24c12.28 12.28 32.2 12.28 44.48 0L176 322.72l100.07 100.07c12.28 12.28 32.2 12.28 44.48 0l22.24-22.24c12.28-12.28 12.28-32.19 0-44.48L242.72 256z"/></svg>' +
            '</button></div>' +
            '<header class="pf-v5-c-modal-box__header"><h1 class="pf-v5-c-modal-box__title" id="pf-confirm-title"></h1></header>' +
            '<div class="pf-v5-c-modal-box__body" id="pf-confirm-body"></div>' +
            '<footer class="pf-v5-c-modal-box__footer">' +
            '<button class="pf-v5-c-button pf-m-primary" id="pf-confirm-ok" onclick="pfConfirmClose(true)">Confirm</button> ' +
            '<button class="pf-v5-c-button pf-m-link" onclick="pfConfirmClose(false)">Cancel</button>' +
            '</footer></div></div></div>';
        document.body.insertAdjacentHTML('beforeend', html);
    }

    window.pfConfirm = function(title, message, opts) {
        ensureConfirmModal();
        document.getElementById('pf-confirm-title').textContent = title;
        document.getElementById('pf-confirm-body').textContent = message;
        var okBtn = document.getElementById('pf-confirm-ok');
        okBtn.textContent = (opts && opts.confirmText) || 'Confirm';
        if (opts && opts.danger) {
            okBtn.className = 'pf-v5-c-button pf-m-danger';
        } else {
            okBtn.className = 'pf-v5-c-button pf-m-primary';
        }
        document.getElementById('pf-confirm-backdrop').style.display = '';
        return new Promise(function(resolve) { _pfConfirmResolve = resolve; });
    };

    window.pfConfirmClose = function(result) {
        document.getElementById('pf-confirm-backdrop').style.display = 'none';
        if (_pfConfirmResolve) {
            _pfConfirmResolve(result);
            _pfConfirmResolve = null;
        }
    };
})();
