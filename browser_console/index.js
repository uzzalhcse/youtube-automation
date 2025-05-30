// Modern Prompt Automation Script - Fixed Version for Browser Console
(function() {
    let prompts = [];
    let currentIndex = 0;
    let isRunning = false;
    let intervalId = null;
    let intervalTime = 10; // Default 10 seconds
    let isVisible = true;

    // Create modern floating box with better UI
    function createFloatingBox() {
        // Remove existing box if present
        const existing = document.getElementById('prompt-automation-box');
        if (existing) existing.remove();

        const floatingDiv = document.createElement('div');
        floatingDiv.id = 'prompt-automation-box';
        floatingDiv.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            width: 450px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            border: none;
            border-radius: 16px;
            padding: 0;
            z-index: 10000;
            box-shadow: 0 20px 40px rgba(0,0,0,0.15), 0 0 0 1px rgba(255,255,255,0.1);
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            max-height: 85vh;
            overflow: hidden;
            backdrop-filter: blur(10px);
            animation: slideIn 0.3s ease-out;
            transition: all 0.3s ease;
        `;

        // Add CSS animations
        const style = document.createElement('style');
        style.textContent = `
            @keyframes slideIn {
                from { transform: translateX(100%); opacity: 0; }
                to { transform: translateX(0); opacity: 1; }
            }
            @keyframes pulse {
                0%, 100% { transform: scale(1); }
                50% { transform: scale(1.05); }
            }
            .pulse-animation { animation: pulse 2s infinite; }
            .collapsed { 
                transform: translateX(calc(100% - 100px)) !important; 
                width: 100px !important;
            }
        `;
        document.head.appendChild(style);

        floatingDiv.innerHTML = `
            <div id="main-content" style="background: rgba(255,255,255,0.95); margin: 2px; border-radius: 14px; backdrop-filter: blur(20px);">
                <!-- Header -->
                <div style="padding: 20px 25px 15px; border-bottom: 1px solid rgba(0,0,0,0.08);">
                    <div style="display: flex; justify-content: space-between; align-items: center;">
                        <div style="display: flex; align-items: center; gap: 10px;">
                            <button id="toggle-visibility" style="background: #10b981; color: white; border: none; border-radius: 8px; width: 32px; height: 32px; cursor: pointer; font-size: 16px; display: flex; align-items: center; justify-content: center; transition: all 0.2s;">+</button>
                            <h3 style="margin: 0; color: #1a1a1a; font-size: 18px; font-weight: 600;">🚀 Prompt Automation</h3>
                        </div>
                    </div>
                </div>

                <!-- Collapsible Content -->
                <div id="collapsible-content" style="display: none;">
                    <!-- Input Section -->
                    <div style="padding: 20px 25px;">
                        <label style="display: block; margin-bottom: 8px; font-weight: 600; color: #2d3748; font-size: 14px;">📝 Paste Prompt String</label>
                        <textarea id="prompt-input" placeholder="Paste your prompt string here..." style="width: 100%; height: 120px; padding: 12px; border: 2px solid #e2e8f0; border-radius: 12px; font-size: 13px; resize: vertical; font-family: 'Monaco', 'Consolas', monospace; background: #f8fafc; transition: all 0.2s; box-sizing: border-box;"></textarea>
                    </div>

                    <!-- Controls Section -->
                    <div style="padding: 0 25px 20px;">
                        <div style="display: flex; gap: 12px; margin-bottom: 15px; align-items: center;">
                            <button id="parse-prompts" style="background: linear-gradient(135deg, #4f46e5, #7c3aed); color: white; border: none; padding: 12px 20px; border-radius: 10px; cursor: pointer; font-weight: 600; font-size: 14px; transition: all 0.2s; flex: 1;">Parse Prompts</button>
                            <div style="display: flex; align-items: center; gap: 8px; background: #f1f5f9; padding: 8px 12px; border-radius: 10px;">
                                <label style="font-size: 12px; color: #475569; font-weight: 500;">Interval:</label>
                                <input id="interval-input" type="number" value="10" min="1" max="300" style="width: 50px; border: 1px solid #cbd5e1; border-radius: 6px; padding: 4px 6px; font-size: 12px; text-align: center;">
                                <span style="font-size: 12px; color: #64748b;">sec</span>
                            </div>
                        </div>
                        
                        <div style="display: flex; gap: 12px; margin-bottom: 15px;">
                            <button id="toggle-automation" style="background: linear-gradient(135deg, #10b981, #059669); color: white; border: none; padding: 12px 24px; border-radius: 10px; cursor: pointer; font-weight: 600; font-size: 14px; transition: all 0.2s; flex: 1;" disabled>
                                <span id="toggle-text">▶️ Start Automation</span>
                            </button>
                            <button id="test-elements" style="background: #6b7280; color: white; border: none; padding: 12px 16px; border-radius: 10px; cursor: pointer; font-size: 12px; transition: all 0.2s;">🔍 Test</button>
                            <button id="test-manual" style="background: #8b5cf6; color: white; border: none; padding: 12px 16px; border-radius: 10px; cursor: pointer; font-size: 12px; transition: all 0.2s;">📝 Test Submit</button>
                        </div>
                    </div>

                    <!-- Status Section -->
                    <div style="padding: 0 25px 20px;">
                        <div id="status" style="padding: 12px 16px; background: linear-gradient(135deg, #f0f9ff, #e0f2fe); border-radius: 10px; font-size: 13px; font-weight: 500; color: #0369a1; border-left: 4px solid #0ea5e9; min-height: 18px;">Ready to parse prompts...</div>
                    </div>

                    <!-- Prompt List Section -->
                    <div style="padding: 0 25px 25px;">
                        <div id="prompt-list-container" style="display: none;">
                            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 12px;">
                                <h4 style="margin: 0; color: #374151; font-size: 14px; font-weight: 600;">📋 Parsed Prompts</h4>
                                <div id="progress-summary" style="font-size: 12px; color: #6b7280; font-weight: 500;"></div>
                            </div>
                            <div id="prompt-list" style="max-height: 250px; overflow-y: auto; border: 1px solid #e5e7eb; border-radius: 10px; background: #fafafa;"></div>
                        </div>
                    </div>
                </div>
            </div>
        `;

        document.body.appendChild(floatingDiv);

        // Add hover effects
        const buttons = floatingDiv.querySelectorAll('button');
        buttons.forEach(btn => {
            btn.addEventListener('mouseenter', () => {
                if (!btn.disabled) btn.style.transform = 'translateY(-2px)';
            });
            btn.addEventListener('mouseleave', () => {
                btn.style.transform = 'translateY(0)';
            });
        });

        return floatingDiv;
    }

    // Parse prompts from input string
    function parsePrompts(inputString) {
        const regex = /Prompt\s*\d+\s*:\s*["""]([^"""]+)["""]/g;
        const parsedPrompts = [];
        let match;

        console.log('Parsing input:', inputString.substring(0, 200) + '...');
        regex.lastIndex = 0;

        while ((match = regex.exec(inputString)) !== null) {
            console.log('Found match:', match[1].substring(0, 50) + '...');
            parsedPrompts.push({
                text: match[1].trim(),
                status: 'pending'
            });
        }

        // Fallback parsing
        if (parsedPrompts.length === 0) {
            console.log('Primary regex failed, trying alternative...');
            const segments = inputString.split(/Prompt\s*\d+\s*:/);

            for (let i = 1; i < segments.length; i++) {
                const segment = segments[i].trim();
                const quotedMatch = segment.match(/["""]([^"""]+)["""]/);
                if (quotedMatch) {
                    parsedPrompts.push({
                        text: quotedMatch[1].trim(),
                        status: 'pending'
                    });
                }
            }
        }

        console.log('Total prompts parsed:', parsedPrompts.length);
        return parsedPrompts;
    }

    // Find form elements with better selectors
    function findFormElements() {
        // Try multiple selectors for textarea
        let textarea = document.querySelector('textarea[placeholder*="message"]') ||
            document.querySelector('textarea[placeholder*="type"]') ||
            document.querySelector('textarea[placeholder*="enter"]') ||
            document.querySelector('textarea[data-testid*="input"]') ||
            document.querySelector('textarea') ||
            document.querySelector('div[contenteditable="true"]') ||
            document.querySelector('input[type="text"]');

        // Try multiple selectors for submit button
        let submitButton = document.querySelector('button[type="submit"]') ||
            document.querySelector('button[aria-label*="send"]') ||
            document.querySelector('button[aria-label*="submit"]') ||
            document.querySelector('button svg[data-icon="send"]') ||
            document.querySelector('button:has(svg)') ||
            Array.from(document.querySelectorAll('button')).find(btn =>
                btn.textContent.toLowerCase().includes('send') ||
                btn.textContent.toLowerCase().includes('submit') ||
                btn.innerHTML.includes('send') ||
                btn.querySelector('svg')
            );

        console.log('Found textarea:', textarea);
        console.log('Found submit button:', submitButton);

        return { textarea, submitButton };
    }

    // Instant text input without typing simulation
    function setTextInstantly(element, text) {
        if (!element) return false;
        // Helper for React-compatible value setting
        function setNativeValue(el, value) {
            const setter = Object.getOwnPropertyDescriptor(el.__proto__, 'value')?.set;
            const prototype = Object.getPrototypeOf(el);
            const prototypeSetter = Object.getOwnPropertyDescriptor(prototype, 'value')?.set;

            if (setter && setter !== prototypeSetter) {
                prototypeSetter.call(el, value);
            } else {
                setter.call(el, value);
            }
        }
        // Handle different input types
        if (element.tagName.toLowerCase() === 'textarea' || element.tagName.toLowerCase() === 'input') {
            // Clear existing content
            element.focus();
            element.select();
            setNativeValue(element, '');
            setNativeValue(element, text);
            element.dispatchEvent(new Event('input', { bubbles: true }));
            // Trigger all necessary events
            const events = ['input', 'change', 'keyup', 'paste'];
            events.forEach(eventType => {
                const event = new Event(eventType, { bubbles: true, cancelable: true });
                element.dispatchEvent(event);
            });

            // React-specific events
            const nativeInputValueSetter = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, "value").set;
            const nativeTextAreaValueSetter = Object.getOwnPropertyDescriptor(window.HTMLTextAreaElement.prototype, "value").set;

            if (element.tagName.toLowerCase() === 'input') {
                nativeInputValueSetter.call(element, text);
            } else {
                nativeTextAreaValueSetter.call(element, text);
            }

            element.dispatchEvent(new Event('input', { bubbles: true }));

        } else if (element.contentEditable === 'true') {
            // Handle contenteditable divs
            element.focus();

            setNativeValue(element, '');
            setNativeValue(element, text);

            const event = new Event('input', { bubbles: true });
            element.dispatchEvent(event);
        }

        return true;
    }

    // Improved submit function
    function submitPrompt(promptIndex) {
        // Validate prompt index
        if (!prompts[promptIndex]) {
            console.error('Invalid prompt index:', promptIndex);
            updateStatus('❌ Error: Invalid prompt index', 'error');
            return false;
        }

        const { textarea, submitButton } = findFormElements();

        if (!textarea || !submitButton) {
            updateStatus('❌ Error: Could not find form elements on page', 'error');
            stopAutomation();
            return false;
        }

        const promptText = prompts[promptIndex].text;
        prompts[promptIndex].status = 'processing';
        updatePromptList();

        // Set text instantly
        if (!setTextInstantly(textarea, promptText)) {
            prompts[promptIndex].status = 'error';
            updatePromptList();
            updateStatus(`❌ Error setting text for prompt ${promptIndex + 1}`, 'error');
            return false;
        }

        // Wait a moment then submit
        setTimeout(() => {
            try {
                // Enable submit button if disabled
                submitButton.disabled = false;
                submitButton.removeAttribute('disabled');

                // Click submit button
                submitButton.click();

                // Update status after a brief delay
                setTimeout(() => {
                    if (prompts[promptIndex]) {
                        prompts[promptIndex].status = 'completed';
                        updatePromptList();
                        updateStatus(`✅ Submitted prompt ${promptIndex + 1}/${prompts.length}`, 'success');
                    }
                }, 1000);

            } catch (error) {
                console.error('Submit error:', error);
                if (prompts[promptIndex]) {
                    prompts[promptIndex].status = 'error';
                    updatePromptList();
                    updateStatus(`❌ Error submitting prompt ${promptIndex + 1}`, 'error');
                }
            }
        }, 500);

        return true;
    }

    // Update status with modern styling
    function updateStatus(message, type = 'info') {
        const statusDiv = document.getElementById('status');
        if (!statusDiv) return;

        const styles = {
            info: {
                background: 'linear-gradient(135deg, #f0f9ff, #e0f2fe)',
                color: '#0369a1',
                border: '#0ea5e9'
            },
            success: {
                background: 'linear-gradient(135deg, #ecfdf5, #d1fae5)',
                color: '#065f46',
                border: '#10b981'
            },
            error: {
                background: 'linear-gradient(135deg, #fef2f2, #fee2e2)',
                color: '#991b1b',
                border: '#ef4444'
            },
            warning: {
                background: 'linear-gradient(135deg, #fffbeb, #fef3c7)',
                color: '#92400e',
                border: '#f59e0b'
            }
        };

        const style = styles[type] || styles.info;
        statusDiv.style.background = style.background;
        statusDiv.style.color = style.color;
        statusDiv.style.borderLeftColor = style.border;
        statusDiv.textContent = message;

        if (type === 'success') {
            statusDiv.classList.add('pulse-animation');
            setTimeout(() => statusDiv.classList.remove('pulse-animation'), 2000);
        }
    }

    // Update prompt list with status colors
    function updatePromptList() {
        const listDiv = document.getElementById('prompt-list');
        const containerDiv = document.getElementById('prompt-list-container');
        const progressDiv = document.getElementById('progress-summary');

        if (!listDiv || prompts.length === 0) return;

        containerDiv.style.display = 'block';

        const completed = prompts.filter(p => p.status === 'completed').length;
        const processing = prompts.filter(p => p.status === 'processing').length;
        const pending = prompts.filter(p => p.status === 'pending').length;
        const errors = prompts.filter(p => p.status === 'error').length;

        progressDiv.innerHTML = `
            <span style="color: #10b981;">✅ ${completed}</span> | 
            <span style="color: #f59e0b;">⏳ ${processing}</span> | 
            <span style="color: #6b7280;">⏸️ ${pending}</span>
            ${errors > 0 ? ' | <span style="color: #ef4444;">❌ ' + errors + '</span>' : ''}
        `;

        listDiv.innerHTML = prompts.map((prompt, index) => {
            const statusColors = {
                pending: { bg: '#f8fafc', border: '#e2e8f0', icon: '⏸️', color: '#64748b' },
                processing: { bg: '#fef3c7', border: '#f59e0b', icon: '⏳', color: '#92400e' },
                completed: { bg: '#ecfdf5', border: '#10b981', icon: '✅', color: '#065f46' },
                error: { bg: '#fef2f2', border: '#ef4444', icon: '❌', color: '#991b1b' }
            };

            const status = statusColors[prompt.status];
            const isActive = index === currentIndex && isRunning;

            return `
                <div style="
                    margin: 8px; 
                    padding: 12px 16px; 
                    background: ${status.bg}; 
                    border-left: 4px solid ${status.border}; 
                    border-radius: 8px;
                    ${isActive ? 'box-shadow: 0 0 0 2px ' + status.border + '40;' : ''}
                    transition: all 0.2s;
                ">
                    <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 6px;">
                        <strong style="color: #374151; font-size: 13px;">Prompt ${index + 1}</strong>
                        <span style="color: ${status.color}; font-size: 12px; font-weight: 600;">
                            ${status.icon} ${prompt.status.toUpperCase()}
                        </span>
                    </div>
                    <div style="color: #6b7280; font-size: 12px; line-height: 1.4; font-family: 'Monaco', 'Consolas', monospace;">
                        ${prompt.text.substring(0, 120)}${prompt.text.length > 120 ? '...' : ''}
                    </div>
                </div>
            `;
        }).join('');
    }

    // Toggle visibility function
    function toggleVisibility() {
        const floatingBox = document.getElementById('prompt-automation-box');
        const toggleBtn = document.getElementById('toggle-visibility');
        const collapsibleContent = document.getElementById('collapsible-content');

        if (isVisible) {
            // Hide content (minimize)
            collapsibleContent.style.display = 'none';
            floatingBox.classList.add('collapsed');
            toggleBtn.textContent = '+';
            toggleBtn.style.background = '#ef4444';
            isVisible = false;
        } else {
            // Show content (maximize)
            collapsibleContent.style.display = 'block';
            floatingBox.classList.remove('collapsed');
            toggleBtn.textContent = '−';
            toggleBtn.style.background = '#10b981';
            isVisible = true;
        }
    }

    // Start automation
    function startAutomation() {
        if (prompts.length === 0) {
            updateStatus('❌ No prompts to submit. Please parse prompts first.', 'error');
            return;
        }

        intervalTime = parseInt(document.getElementById('interval-input').value) || 10;
        isRunning = true;
        currentIndex = 0;

        // Reset all prompts to pending
        prompts.forEach(prompt => prompt.status = 'pending');

        updateToggleButton();
        updateStatus(`🚀 Starting automation with ${intervalTime}s intervals...`, 'info');

        // Submit first prompt immediately
        if (submitPrompt(currentIndex)) {
            currentIndex++;
            // Set interval for remaining prompts
            if (currentIndex < prompts.length) {
                intervalId = setInterval(() => {
                    if (currentIndex >= prompts.length) {
                        stopAutomation();
                        updateStatus('🎉 All prompts submitted successfully!', 'success');
                        return;
                    }
                    if (submitPrompt(currentIndex)) {
                        currentIndex++;
                    } else {
                        stopAutomation();
                    }
                }, intervalTime * 1000);
            } else {
                stopAutomation();
                updateStatus('🎉 All prompts submitted successfully!', 'success');
            }
        }
    }

    // Stop automation
    function stopAutomation() {
        isRunning = false;
        if (intervalId) {
            clearInterval(intervalId);
            intervalId = null;
        }
        updateToggleButton();
        if (currentIndex < prompts.length) {
            updateStatus('⏹️ Automation stopped', 'warning');
        }
    }

    // Update toggle button appearance
    function updateToggleButton() {
        const toggleBtn = document.getElementById('toggle-automation');
        const toggleText = document.getElementById('toggle-text');

        if (isRunning) {
            toggleBtn.style.background = 'linear-gradient(135deg, #ef4444, #dc2626)';
            toggleText.textContent = '⏹️ Stop Automation';
        } else {
            toggleBtn.style.background = 'linear-gradient(135deg, #10b981, #059669)';
            toggleText.textContent = '▶️ Start Automation';
        }
    }

    // Initialize the floating box
    const floatingBox = createFloatingBox();

    // Event listeners
    document.getElementById('toggle-visibility').addEventListener('click', toggleVisibility);

    document.getElementById('parse-prompts').addEventListener('click', () => {
        const inputText = document.getElementById('prompt-input').value.trim();
        if (!inputText) {
            updateStatus('❌ Please enter prompt text to parse', 'error');
            return;
        }

        const parsedPrompts = parsePrompts(inputText);

        if (parsedPrompts.length === 0) {
            updateStatus('❌ No valid prompts found. Check your input format.', 'error');
        } else {
            prompts = parsedPrompts;
            currentIndex = 0;
            updateStatus(`✅ Found ${prompts.length} prompts`, 'success');
            document.getElementById('toggle-automation').disabled = false;
            updatePromptList();
        }
    });

    document.getElementById('toggle-automation').addEventListener('click', () => {
        if (isRunning) {
            stopAutomation();
        } else {
            startAutomation();
        }
    });

    document.getElementById('test-elements').addEventListener('click', () => {
        const { textarea, submitButton } = findFormElements();
        updateStatus(`🔍 Found textarea: ${!!textarea}, Found submit button: ${!!submitButton}`, 'info');

        if (textarea) console.log('Textarea element:', textarea);
        if (submitButton) console.log('Submit button element:', submitButton);
    });

    document.getElementById('test-manual').addEventListener('click', () => {
        const { textarea, submitButton } = findFormElements();

        if (!textarea || !submitButton) {
            updateStatus('❌ Elements not found for test', 'error');
            return;
        }

        const testText = "Test prompt from automation script";

        if (setTextInstantly(textarea, testText)) {
            setTimeout(() => {
                try {
                    submitButton.click();
                    updateStatus('🧪 Test submission completed!', 'success');
                } catch (error) {
                    updateStatus('❌ Test submission failed', 'error');
                    console.error('Test submit error:', error);
                }
            }, 500);
        } else {
            updateStatus('❌ Failed to set test text', 'error');
        }
    });

    // Interval input change handler
    document.getElementById('interval-input').addEventListener('change', (e) => {
        intervalTime = parseInt(e.target.value) || 10;
        updateStatus(`⏱️ Interval set to ${intervalTime} seconds`, 'info');
    });

    // Initialize in minimized state
    toggleVisibility();

    updateStatus('🎯 Modern Prompt Automation ready! Click + to expand and parse your prompts.', 'info');
    console.log('🚀 Modern Prompt Automation Script loaded successfully!');
    console.log('A modern floating interface should appear in the top-right corner (minimized).');
})();