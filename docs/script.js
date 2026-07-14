document.addEventListener('DOMContentLoaded', () => {
    // DOM Elements
    const hamburgerBtn = document.getElementById('hamburger-btn');
    const navMenu = document.getElementById('nav-menu');
    const settingsBtn = document.getElementById('settings-btn');
    const settingsContent = document.getElementById('settings-content');
    const themeToggle = document.getElementById('theme-toggle');
    const colorblindToggle = document.getElementById('colorblind-toggle');
    const body = document.body;

    // Mobile Hamburger Menu Toggle
    hamburgerBtn.addEventListener('click', () => {
        hamburgerBtn.classList.toggle('active');
        navMenu.classList.toggle('active');
    });

    // Close mobile menu when clicking a link
    document.querySelectorAll('.nav-link').forEach(link => {
        link.addEventListener('click', () => {
            hamburgerBtn.classList.remove('active');
            navMenu.classList.remove('active');
        });
    });

    // Settings Dropdown Toggle
    settingsBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        settingsContent.classList.toggle('show');
    });

    // Close settings dropdown when clicking outside
    document.addEventListener('click', (e) => {
        if (!settingsBtn.contains(e.target) && !settingsContent.contains(e.target)) {
            settingsContent.classList.remove('show');
        }
    });

    // Preferences State Management
    let isDarkMode = localStorage.getItem('darkMode') === 'true';
    let isColorBlindMode = localStorage.getItem('colorBlindMode') === 'true';

    // Apply initial state
    if (isDarkMode) {
        body.classList.replace('light-mode', 'dark-mode');
        themeToggle.textContent = 'Light Mode';
    }

    if (isColorBlindMode) {
        body.classList.add('color-blind-mode');
        colorblindToggle.textContent = 'Disable CB Mode';
    }

    // Theme Toggle Logic
    themeToggle.addEventListener('click', () => {
        if (body.classList.contains('light-mode')) {
            body.classList.replace('light-mode', 'dark-mode');
            themeToggle.textContent = 'Light Mode';
            localStorage.setItem('darkMode', 'true');
        } else {
            body.classList.replace('dark-mode', 'light-mode');
            themeToggle.textContent = 'Dark Mode';
            localStorage.setItem('darkMode', 'false');
        }
    });

    // Color Blind Mode Logic
    colorblindToggle.addEventListener('click', () => {
        body.classList.toggle('color-blind-mode');
        if (body.classList.contains('color-blind-mode')) {
            colorblindToggle.textContent = 'Disable CB Mode';
            localStorage.setItem('colorBlindMode', 'true');
        } else {
            colorblindToggle.textContent = 'Color Blind Mode';
            localStorage.setItem('colorBlindMode', 'false');
        }
    });

    // Intersection Observer for scroll animations (fade-in)
    const fadeElements = document.querySelectorAll('.fade-in');
    
    // Elements are already animated on load due to CSS animations, 
    // but this observer would handle scroll-based reveal if they were lower down the page.
    const observer = new IntersectionObserver((entries) => {
        entries.forEach(entry => {
            if (entry.isIntersecting) {
                entry.target.style.animationPlayState = 'running';
                observer.unobserve(entry.target);
            }
        });
    }, {
        threshold: 0.1
    });

    fadeElements.forEach(el => {
        // We pause the animation initially if it's below the fold
        if (el.getBoundingClientRect().top > window.innerHeight) {
            el.style.animationPlayState = 'paused';
            observer.observe(el);
        }
    });
});
