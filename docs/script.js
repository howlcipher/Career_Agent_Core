document.addEventListener('DOMContentLoaded', () => {
    // --- Theme Toggle ---
    const themeToggle = document.getElementById('theme-toggle');
    const htmlElement = document.documentElement;
    
    const savedTheme = localStorage.getItem('theme');
    if (savedTheme) {
        htmlElement.setAttribute('data-theme', savedTheme);
    } else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: light)').matches) {
        htmlElement.setAttribute('data-theme', 'light');
    }

    if (themeToggle) {
        themeToggle.addEventListener('click', () => {
            const currentTheme = htmlElement.getAttribute('data-theme');
            const newTheme = currentTheme === 'dark' ? 'light' : 'dark';
            htmlElement.setAttribute('data-theme', newTheme);
            localStorage.setItem('theme', newTheme);
        });
    }

    // --- Mobile Navigation Toggle ---
    const mobileToggle = document.querySelector('.mobile-toggle');
    const navMenu = document.querySelector('.nav-menu');

    if (mobileToggle && navMenu) {
        mobileToggle.addEventListener('click', () => {
            const isExpanded = mobileToggle.getAttribute('aria-expanded') === 'true' || false;
            mobileToggle.setAttribute('aria-expanded', !isExpanded);
            mobileToggle.classList.toggle('active');
            navMenu.classList.toggle('active');
        });

        const navLinks = document.querySelectorAll('.nav-link, .btn-nav');
        navLinks.forEach(link => {
            link.addEventListener('click', () => {
                mobileToggle.setAttribute('aria-expanded', 'false');
                mobileToggle.classList.remove('active');
                navMenu.classList.remove('active');
            });
        });
    }

    // --- Scroll Animations (Intersection Observer) ---
    const fadeElements = document.querySelectorAll('.fade-in-up');
    const observerOptions = {
        root: null,
        rootMargin: '0px',
        threshold: 0.1
    };

    const scrollObserver = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                entry.target.classList.add('visible');
            }
        });
    }, observerOptions);

    fadeElements.forEach(el => scrollObserver.observe(el));

    // --- Parallax Effect on Ambient Orbs ---
    const ambientOrbs = document.querySelectorAll('.ambient-orb');
    let mouseX = window.innerWidth / 2;
    let mouseY = window.innerHeight / 2;
    let currentX = mouseX;
    let currentY = mouseY;
    let isAnimating = true;
    
    window.addEventListener('mousemove', (e) => {
        mouseX = e.clientX;
        mouseY = e.clientY;
        if (!isAnimating) {
            isAnimating = true;
            requestAnimationFrame(animateOrbs);
        }
    }, { passive: true });

    function animateOrbs() {
        const dx = mouseX - currentX;
        const dy = mouseY - currentY;

        if (Math.abs(dx) < 0.1 && Math.abs(dy) < 0.1) {
            isAnimating = false;
            return;
        }

        // Smooth interpolation for parallax
        currentX += dx * 0.05;
        currentY += dy * 0.05;

        const x = currentX / window.innerWidth;
        const y = currentY / window.innerHeight;

        ambientOrbs.forEach((orb, index) => {
            const speed = (index + 1) * 30;
            const xOffset = (x - 0.5) * speed;
            const yOffset = (y - 0.5) * speed;
            orb.style.transform = `translate3d(${xOffset}px, ${yOffset}px, 0)`;
        });
        requestAnimationFrame(animateOrbs);
    }
    animateOrbs();

    // --- Bento Card Spotlight Effect ---
    const cards = document.querySelectorAll('.bento-card');
    cards.forEach(card => {
        card.addEventListener('mousemove', (e) => {
            const rect = card.getBoundingClientRect();
            const x = e.clientX - rect.left;
            const y = e.clientY - rect.top;
            card.style.setProperty('--mouse-x', `${x}px`);
            card.style.setProperty('--mouse-y', `${y}px`);
        });
    });

    // --- Scrolled Nav Effect ---
    const nav = document.querySelector('.glass-nav');
    if (nav) {
        const navObserver = new IntersectionObserver(
            ([e]) => nav.classList.toggle('scrolled', !e.isIntersecting)
        );
        const sentinel = document.createElement('div');
        sentinel.style.position = 'absolute';
        sentinel.style.top = '50px';
        sentinel.style.height = '1px';
        sentinel.style.width = '100%';
        document.body.prepend(sentinel);
        navObserver.observe(sentinel);
    }
});
