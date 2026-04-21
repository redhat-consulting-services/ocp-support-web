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
})();
