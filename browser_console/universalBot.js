// Unified Text Extraction and Auto-Continue Bot System
function createUnifiedBotSystem(config = {}) {
    // Default configuration
    const defaultConfig = {
        extraction: {
            maxChars: 9050,
            selector: '[data-message-author-role="assistant"]',
            markdownSelector: '.markdown.prose',
            paragraphSelector: 'p:not(blockquote p)',
            cleaningPatterns: {
                // More comprehensive word count patterns
                wordCount: /\[\s*Word\s+count:\s*[\d,]+\s*\]/gi,
                wordCountAlt: /Word\s+count:\s*[\d,]+/gi,
                // More comprehensive section patterns
                sections: /Section\s+\d+/gi,
                sectionStart: /^Section\s+\d+\s*/gmi,
                // End markers
                awaitingContinue: /Awaiting\s+"?CONTINUE"?/gi,
                // Additional patterns
                standalone: /\*\*\[Word\s+count:\s*[\d,]+\]\s*\*\*/gi,
                bracketed: /\[\s*[\d,]+\s*\]/gi,
                customPatterns: []
            },
            outputFormat: 'json',
            logVerbose: true
        },
        autoContinue: {
            intervalMinutes: 1,
            continueText: 'CONTINUE',
            endCondition: 'End of script',
            minEndOccurrences: 3,
            textAreaSelector: '#prompt-textarea',
            submitButtonSelector: '#composer-submit-button',
            enabled: false
        },
        ui: {
            position: 'right',
            showExtraction: true,
            showAutoContinue: true
        }
    };

    // Merge configurations
    const settings = JSON.parse(JSON.stringify(defaultConfig));
    if (config.extraction) {
        Object.assign(settings.extraction, config.extraction);
        if (config.extraction.cleaningPatterns) {
            Object.assign(settings.extraction.cleaningPatterns, config.extraction.cleaningPatterns);
        }
    }
    if (config.autoContinue) {
        Object.assign(settings.autoContinue, config.autoContinue);
    }
    if (config.ui) {
        Object.assign(settings.ui, config.ui);
    }

    // Global state
    let autoSubmitInterval = null;
    let extractedChunks = [];
    let cleanedText = '';
    let isAutoRunning = false;

    // Utility Functions
    function log(message, type = 'info') {
        if (settings.extraction.logVerbose) {
            const emoji = type === 'error' ? '❌' : type === 'success' ? '✅' : type === 'warning' ? '⚠️' : '📍';
            console.log(`${emoji} [UnifiedBot] ${message}`);
        }
    }

    function updateUI() {
        const container = document.getElementById('unified-bot-ui');
        if (container) {
            updateUIContent();
        } else {
            createUI();
        }
    }

    // Enhanced Text Cleaning Function
    function cleanText(text) {
        let cleanedText = text;

        log(`Original text length: ${text.length}`);
        log(`Original text sample: "${text.substring(0, 300)}..."`);

        // AGGRESSIVE CLEANING - Apply multiple passes with different patterns

        // Pass 1: Remove all word count patterns (most comprehensive)
        cleanedText = cleanedText
            .replace(/\*\*\[Word\s+count:\s*[\d,]+\]\s*\*\*/gi, '') // **[Word count: 1,098]**
            .replace(/\[\s*Word\s+count:\s*[\d,]+\s*\]/gi, '')      // [Word count: 1,098]
            .replace(/Word\s+count:\s*[\d,]+/gi, '')                // Word count: 1,098
            .replace(/\*\*\[\s*[\d,]+\s*\]\*\*/gi, '')             // **[1,098]**
            .replace(/\[\s*[\d,]+\s*\]/gi, '')                     // [1,098]
            .replace(/\*\*Word\s+count:\s*[\d,]+\*\*/gi, '');      // **Word count: 1,098**

        log(`After word count cleaning: "${cleanedText.substring(0, 300)}..."`);

        // Pass 2: Remove section markers
        cleanedText = cleanedText
            .replace(/Section\s+\d+/gi, '')                        // Section 9
            .replace(/^Section\s+\d+\s*/gmi, '')                   // Section 9 at start of line
            .replace(/\*\*Section\s+\d+\*\*/gi, '')               // **Section 9**
            .replace(/Section\s+\d+\s*:/gi, '');                   // Section 9:

        log(`After section cleaning: "${cleanedText.substring(0, 300)}..."`);

        // Pass 3: Remove other common patterns
        cleanedText = cleanedText
            .replace(/Awaiting\s+"?CONTINUE"?/gi, '')              // Awaiting CONTINUE
            .replace(/\*\*\*+/g, '')                               // Multiple asterisks
            .replace(/---+/g, '')                                  // Multiple dashes
            .replace(/===+/g, '');                                 // Multiple equals

        // Pass 4: Clean up whitespace aggressively
        cleanedText = cleanedText
            .replace(/\s*\*\*\s*/g, ' ')        // Remove ** with surrounding spaces
            .replace(/\s+/g, ' ')               // Multiple spaces to single
            .replace(/\n\s*\n/g, '\n')          // Multiple newlines to single
            .replace(/^\s+|\s+$/g, '')          // Trim start and end
            .replace(/\s+([.!?])/g, '$1')       // Remove space before punctuation
            .replace(/([.!?])\s*([A-Z])/g, '$1 $2') // Ensure space after sentence end
            .replace(/\s*,\s*/g, ', ')          // Normalize comma spacing
            .replace(/\s*;\s*/g, '; ')          // Normalize semicolon spacing
            .replace(/\s*:\s*/g, ': ');         // Normalize colon spacing

        // Pass 5: Final cleanup - remove any remaining isolated numbers or brackets
        cleanedText = cleanedText
            .replace(/\s+\d+\s+/g, ' ')         // Remove isolated numbers
            .replace(/\[\s*\]/g, '')            // Remove empty brackets
            .replace(/\(\s*\)/g, '')            // Remove empty parentheses
            .replace(/\s{2,}/g, ' ')            // Final space normalization
            .trim();

        log(`Final cleaned text length: ${cleanedText.length}`);
        log(`Final cleaned text sample: "${cleanedText.substring(0, 300)}..."`);

        return cleanedText;
    }

    // Text Extraction Functions
    function extractChatGPTReplies() {
        const chatGPTMessages = document.querySelectorAll(settings.extraction.selector);
        let allText = '';
        let processedMessages = 0;

        log(`Found ${chatGPTMessages.length} ChatGPT messages`);

        chatGPTMessages.forEach((message, index) => {
            const markdownDiv = message.querySelector(settings.extraction.markdownSelector);
            if (markdownDiv) {
                let messageText = markdownDiv.textContent.trim();

                // AGGRESSIVE pre-cleaning before section detection
                let tempCleanedText = messageText
                    .replace(/\*\*\[Word\s+count:\s*[\d,]+\]\s*\*\*/gi, '')     // **[Word count: 1,098]**
                    .replace(/\[\s*Word\s+count:\s*[\d,]+\s*\]/gi, '')          // [Word count: 1,098]
                    .replace(/Word\s+count:\s*[\d,]+/gi, '')                    // Word count: 1,098
                    .replace(/\*\*\[\s*[\d,]+\s*\]\*\*/gi, '')                 // **[1,098]**
                    .replace(/\[\s*[\d,]+\s*\]/gi, '')                         // [1,098]
                    .replace(/\*\*Word\s+count:\s*[\d,]+\*\*/gi, '')           // **Word count: 1,098**
                    .replace(/\*\*\s*\*\*/gi, '')                              // Empty bold markers
                    .replace(/\s+/g, ' ')                                       // Multiple spaces
                    .trim();

                log(`Message ${index + 1} before cleaning: "${messageText.substring(0, 100)}..."`);
                log(`Message ${index + 1} after pre-cleaning: "${tempCleanedText.substring(0, 100)}..."`);

                const startsWithSection = /^Section\s+\d+/i.test(tempCleanedText);

                if (startsWithSection) {
                    // Extract all text content, then clean it
                    let messageContent = '';
                    const paragraphs = markdownDiv.querySelectorAll(settings.extraction.paragraphSelector);
                    paragraphs.forEach(p => {
                        const text = p.textContent.trim();
                        if (text) {
                            messageContent += text + ' ';
                        }
                    });

                    // If no paragraphs found, get all text content
                    if (!messageContent.trim()) {
                        messageContent = markdownDiv.textContent.trim();
                    }

                    allText += messageContent + ' ';
                    processedMessages++;
                    log(`Processed message ${index + 1}: Starts with Section`);
                } else {
                    log(`Skipped message ${index + 1}: Doesn't start with Section - "${tempCleanedText.substring(0, 50)}..."`);
                }
            }
        });

        log(`Processed ${processedMessages} out of ${chatGPTMessages.length} messages`);
        log(`Raw extracted text sample: "${allText.substring(0, 300)}..."`);
        return allText.trim();
    }

    function chunkTextBySentences(text, maxChars) {
        const sentences = text.match(/[^\.!?]+[\.!?]+/g) || [text];
        const chunks = [];
        let currentChunk = '';

        for (let sentence of sentences) {
            sentence = sentence.trim();
            if (!sentence) continue;

            if (currentChunk.length + sentence.length + 1 > maxChars) {
                if (currentChunk) {
                    chunks.push(currentChunk.trim());
                    currentChunk = '';
                }

                if (sentence.length > maxChars) {
                    const words = sentence.split(' ');
                    let tempChunk = '';

                    for (let word of words) {
                        if (tempChunk.length + word.length + 1 <= maxChars) {
                            tempChunk += (tempChunk ? ' ' : '') + word;
                        } else {
                            if (tempChunk) {
                                chunks.push(tempChunk);
                                tempChunk = word;
                            } else {
                                chunks.push(word);
                                tempChunk = '';
                            }
                        }
                    }

                    if (tempChunk) {
                        currentChunk = tempChunk;
                    }
                } else {
                    currentChunk = sentence;
                }
            } else {
                currentChunk += (currentChunk ? ' ' : '') + sentence;
            }
        }

        if (currentChunk) {
            chunks.push(currentChunk.trim());
        }

        return chunks.filter(chunk => chunk.length > 0);
    }

    // Auto-Continue Functions
    function checkForEndCondition() {
        const bodyText = document.body.innerText || document.body.textContent || '';
        const matches = (bodyText.match(new RegExp(settings.autoContinue.endCondition, 'g')) || []).length;
        log(`Found ${matches} occurrences of "${settings.autoContinue.endCondition}"`);
        return matches >= settings.autoContinue.minEndOccurrences;
    }

    function typeAndSubmit() {
        try {
            if (checkForEndCondition()) {
                log(`"${settings.autoContinue.endCondition}" detected! Stopping auto-submit...`, 'warning');
                stopAutoSubmit();
                return false;
            }

            const textArea = document.querySelector(settings.autoContinue.textAreaSelector);
            if (!textArea) {
                log('Text area not found', 'error');
                return false;
            }

            textArea.innerHTML = `<p>${settings.autoContinue.continueText}</p>`;
            textArea.dispatchEvent(new Event('input', { bubbles: true }));
            textArea.dispatchEvent(new Event('change', { bubbles: true }));

            setTimeout(() => {
                const submitButton = document.querySelector(settings.autoContinue.submitButtonSelector);
                if (submitButton && !submitButton.disabled) {
                    submitButton.click();
                    log(`Submitted "${settings.autoContinue.continueText}" at ${new Date().toLocaleTimeString()}`, 'success');
                    updateUI();
                    return true;
                } else {
                    log('Submit button not found or disabled', 'error');
                    return false;
                }
            }, 100);

        } catch (error) {
            log(`Error in typeAndSubmit: ${error.message}`, 'error');
            return false;
        }
    }

    function startAutoSubmit() {
        if (isAutoRunning) {
            log('Auto-submit already running', 'warning');
            return;
        }

        if (checkForEndCondition()) {
            log(`"${settings.autoContinue.endCondition}" already detected! Not starting auto-submit.`, 'warning');
            return;
        }

        log(`Starting auto-submit every ${settings.autoContinue.intervalMinutes} minutes...`);
        isAutoRunning = true;

        typeAndSubmit();

        autoSubmitInterval = setInterval(() => {
            typeAndSubmit();
        }, settings.autoContinue.intervalMinutes * 60000);

        updateUI();
        log('Auto-submit started', 'success');
    }

    function stopAutoSubmit() {
        if (autoSubmitInterval) {
            clearInterval(autoSubmitInterval);
            autoSubmitInterval = null;
            isAutoRunning = false;
            log('Auto-submit stopped', 'success');
            updateUI();
        }
    }

    // UI Functions
    function createUI() {
        const existingUI = document.getElementById('unified-bot-ui');
        if (existingUI) {
            existingUI.remove();
        }

        const container = document.createElement('div');
        container.id = 'unified-bot-ui';
        container.innerHTML = getUIHTML();

        document.body.appendChild(container);
        attachEventListeners();
        updateUIContent();
    }

    function attachEventListeners() {
        // Close button
        document.getElementById('close-bot-ui').addEventListener('click', () => {
            document.getElementById('unified-bot-ui').remove();
        });

        // Auto-continue controls
        document.getElementById('start-auto-btn').addEventListener('click', startAutoSubmit);
        document.getElementById('stop-auto-btn').addEventListener('click', stopAutoSubmit);
        document.getElementById('manual-continue-btn').addEventListener('click', typeAndSubmit);

        // Extraction controls
        document.getElementById('extract-btn').addEventListener('click', performExtraction);
        document.getElementById('copy-all-btn').addEventListener('click', copyAllChunks);

        // Settings
        document.getElementById('toggle-settings-btn').addEventListener('click', toggleSettings);
        document.getElementById('apply-settings-btn').addEventListener('click', applySettings);
    }

    function updateUIContent() {
        const container = document.getElementById('unified-bot-ui');
        if (!container) return;

        // Update auto-continue status
        const autoStatus = document.getElementById('auto-status');
        if (autoStatus) {
            autoStatus.textContent = isAutoRunning ? 'Running' : 'Stopped';
            autoStatus.style.color = isAutoRunning ? '#4CAF50' : '#f44336';
        }

        // Update last check
        const lastCheck = document.getElementById('last-check');
        if (lastCheck) {
            lastCheck.textContent = new Date().toLocaleTimeString();
        }

        // Update extraction info
        const chunkCount = document.getElementById('chunk-count');
        const totalChars = document.getElementById('total-chars');
        if (chunkCount) chunkCount.textContent = extractedChunks.length;
        if (totalChars) totalChars.textContent = cleanedText.length.toLocaleString();

        // Update chunks display
        updateChunksDisplay();
    }

    function updateChunksDisplay() {
        const container = document.getElementById('chunks-container');
        if (!container || extractedChunks.length === 0) {
            if (container) container.innerHTML = '<p>No chunks available. Click "Extract" to generate chunks.</p>';
            return;
        }

        container.innerHTML = extractedChunks.map((chunk, index) => `
            <div class="chunk-item">
                <div class="chunk-header">
                    <span class="chunk-title">Chunk ${index + 1}</span>
                    <div class="chunk-actions">
                        <span class="chunk-size">${chunk.length} chars</span>
                        <button class="copy-chunk-btn" data-index="${index}">📄 Copy</button>
                    </div>
                </div>
                <div class="chunk-preview">${chunk.substring(0, 200)}${chunk.length > 200 ? '...' : ''}</div>
            </div>
        `).join('');

        // Attach copy handlers
        container.querySelectorAll('.copy-chunk-btn').forEach(btn => {
            btn.addEventListener('click', (e) => {
                const index = parseInt(e.target.dataset.index);
                copyToClipboard(extractedChunks[index], btn);
            });
        });
    }

    // Action Functions
    function performExtraction() {
        try {
            log('Starting text extraction...');
            const rawText = extractChatGPTReplies();
            log(`Raw extracted text sample: "${rawText.substring(0, 200)}..."`);

            cleanedText = cleanText(rawText);
            extractedChunks = chunkTextBySentences(cleanedText, settings.extraction.maxChars);

            log(`Created ${extractedChunks.length} chunks from ${cleanedText.length} characters`, 'success');

            // Log first chunk sample for debugging
            if (extractedChunks.length > 0) {
                log(`First chunk sample: "${extractedChunks[0].substring(0, 100)}..."`);
            }

            updateUI();

        } catch (error) {
            log(`Extraction error: ${error.message}`, 'error');
        }
    }

    function copyAllChunks() {
        if (extractedChunks.length === 0) {
            log('No chunks to copy. Extract text first.', 'warning');
            return;
        }

        // MODIFIED: Merge all chunks into a single string instead of JSON array
        const mergedText = extractedChunks.join(' ');
        copyToClipboard(mergedText, document.getElementById('copy-all-btn'));
        log(`Copied merged text: ${mergedText.length} characters`, 'success');
    }

    function copyToClipboard(text, button) {
        navigator.clipboard.writeText(text).then(() => {
            const originalText = button.textContent;
            button.textContent = '✅ Copied!';
            setTimeout(() => {
                button.textContent = originalText;
            }, 2000);
            log('Copied to clipboard', 'success');
        }).catch(() => {
            log('Failed to copy to clipboard', 'error');
        });
    }

    function toggleSettings() {
        const panel = document.getElementById('settings-panel');
        panel.style.display = panel.style.display === 'none' ? 'block' : 'none';
    }

    function applySettings() {
        const continueText = document.getElementById('continue-text-input').value;
        const interval = parseInt(document.getElementById('interval-input').value);
        const chunkSize = parseInt(document.getElementById('chunk-size-input').value);

        settings.autoContinue.continueText = continueText;
        settings.autoContinue.intervalMinutes = interval;
        settings.extraction.maxChars = chunkSize;

        if (isAutoRunning) {
            stopAutoSubmit();
            startAutoSubmit();
        }

        log('Settings applied', 'success');
        updateUI();
    }

    function getUIHTML() {
        return `
    <style>
        #unified-bot-ui {
            position: fixed;
            top: 24px;
            right: 24px;
            width: 380px;
            background: rgba(255, 255, 255, 0.95);
            backdrop-filter: blur(20px);
            border: 1px solid rgba(0, 0, 0, 0.08);
            border-radius: 16px;
            box-shadow: 0 20px 40px rgba(0, 0, 0, 0.12), 0 0 0 1px rgba(255, 255, 255, 0.5) inset;
            z-index: 10000;
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            font-size: 14px;
            max-height: 85vh;
            overflow: hidden;
            transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
        }

        #unified-bot-ui:hover {
            box-shadow: 0 24px 48px rgba(0, 0, 0, 0.15), 0 0 0 1px rgba(255, 255, 255, 0.6) inset;
        }

        #unified-bot-container {
            padding: 0;
            overflow-y: auto;
            max-height: 85vh;
        }

        /* Custom scrollbar */
        #unified-bot-container::-webkit-scrollbar {
            width: 6px;
        }
        
        #unified-bot-container::-webkit-scrollbar-track {
            background: rgba(0, 0, 0, 0.02);
        }
        
        #unified-bot-container::-webkit-scrollbar-thumb {
            background: rgba(0, 0, 0, 0.15);
            border-radius: 3px;
        }

        #bot-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            padding: 20px 24px 16px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            margin: 0;
            border-radius: 16px 16px 0 0;
            color: white;
        }

        #bot-title {
            margin: 0;
            font-size: 18px;
            font-weight: 600;
            letter-spacing: -0.02em;
        }

        #bot-status {
            margin: 4px 0 0 0;
            opacity: 0.85;
            font-size: 13px;
            font-weight: 400;
        }

        #close-bot-ui {
            background: rgba(255, 255, 255, 0.2);
            backdrop-filter: blur(10px);
            color: white;
            border: none;
            border-radius: 50%;
            width: 32px;
            height: 32px;
            cursor: pointer;
            font-size: 18px;
            display: flex;
            align-items: center;
            justify-content: center;
            transition: all 0.2s ease;
        }

        #close-bot-ui:hover {
            background: rgba(255, 255, 255, 0.3);
            transform: scale(1.05);
        }

        .control-btn {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            border: none;
            padding: 8px 16px;
            margin: 4px;
            border-radius: 8px;
            cursor: pointer;
            font-size: 12px;
            font-weight: 500;
            transition: all 0.2s ease;
            box-shadow: 0 2px 8px rgba(102, 126, 234, 0.3);
        }

        .control-btn:hover {
            transform: translateY(-1px);
            box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
        }

        .control-btn:active {
            transform: translateY(0);
        }

        .control-btn.secondary {
            background: linear-gradient(135deg, #4CAF50 0%, #45a049 100%);
            box-shadow: 0 2px 8px rgba(76, 175, 80, 0.3);
        }

        .control-btn.secondary:hover {
            box-shadow: 0 4px 12px rgba(76, 175, 80, 0.4);
        }

        .control-btn.accent {
            background: linear-gradient(135deg, #FF6B6B 0%, #EE5A52 100%);
            box-shadow: 0 2px 8px rgba(255, 107, 107, 0.3);
        }

        .control-btn.accent:hover {
            box-shadow: 0 4px 12px rgba(255, 107, 107, 0.4);
        }

        .section {
            padding: 20px 24px;
            border-bottom: 1px solid rgba(0, 0, 0, 0.06);
        }

        .section:last-child {
            border-bottom: none;
        }

        .section-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 16px;
        }

        .section-title {
            margin: 0;
            font-size: 16px;
            font-weight: 600;
            color: #2c3e50;
            letter-spacing: -0.01em;
        }

        .controls-group {
            display: flex;
            gap: 4px;
        }

        .info-grid {
            display: grid;
            grid-template-columns: 1fr 1fr;
            gap: 12px;
            margin-bottom: 16px;
        }

        .info-card {
            background: rgba(102, 126, 234, 0.05);
            border: 1px solid rgba(102, 126, 234, 0.1);
            border-radius: 12px;
            padding: 12px;
            text-align: center;
            transition: all 0.2s ease;
        }

        .info-card:hover {
            background: rgba(102, 126, 234, 0.08);
            transform: translateY(-1px);
        }

        .info-label {
            font-size: 11px;
            color: #6b7280;
            font-weight: 500;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin-bottom: 4px;
        }

        .info-value {
            font-size: 14px;
            font-weight: 600;
            color: #1f2937;
        }

        .status-indicator {
            display: inline-flex;
            align-items: center;
            gap: 6px;
        }

        .status-dot {
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: #ef4444;
            animation: pulse 2s infinite;
        }

        .status-dot.active {
            background: #22c55e;
        }

        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }

        #chunks-container {
            max-height: 240px;
            overflow-y: auto;
            padding-right: 8px;
        }

        .chunk-item {
            background: transparent;
            border: 1px solid rgba(0, 0, 0, 0.08);
            border-radius: 12px;
            margin-bottom: 12px;
            padding: 16px;
            transition: all 0.2s ease;
            box-shadow: 0 2px 4px rgba(0, 0, 0, 0.04);
        }

        .chunk-item:hover {
            border-color: rgba(102, 126, 234, 0.2);
            box-shadow: 0 4px 12px rgba(0, 0, 0, 0.08);
            transform: translateY(-1px);
        }

        .chunk-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 8px;
        }

        .chunk-title {
            font-weight: 600;
            font-size: 13px;
        }

        .chunk-actions {
            display: flex;
            align-items: center;
            gap: 8px;
        }

        .chunk-size {
            font-size: 10px;
            color: #edc721;
            background: rgba(107, 114, 128, 0.1);
            padding: 2px 6px;
            border-radius: 4px;
        }

        .copy-chunk-btn {
            background: linear-gradient(135deg, #10b981 0%, #059669 100%);
            color: white;
            border: none;
            padding: 4px 8px;
            border-radius: 6px;
            cursor: pointer;
            font-size: 10px;
            font-weight: 500;
            transition: all 0.2s ease;
        }

        .copy-chunk-btn:hover {
            transform: scale(1.05);
        }

        .chunk-preview {
            font-size: 12px;
            color: #4b5563;
            background: rgba(0, 0, 0, 0.02);
            padding: 12px;
            border-radius: 8px;
            line-height: 1.5;
            border-left: 3px solid rgba(102, 126, 234, 0.3);
        }

        .settings-panel {
            background: rgba(0, 0, 0, 0.02);
            border-radius: 12px;
            padding: 20px;
            margin-top: 16px;
            border: 1px solid rgba(0, 0, 0, 0.06);
        }

        .setting-group {
            margin-bottom: 16px;
        }

        .setting-group label {
            display: block;
            margin-bottom: 6px;
            font-size: 13px;
            font-weight: 500;
        }

        .setting-group input {
            width: 100%;
            padding: 10px 12px;
            border: 1px solid rgba(0, 0, 0, 0.12);
            border-radius: 8px;
            font-size: 13px;
            background: transparent;
            transition: all 0.2s ease;
            box-sizing: border-box;
        }

        .setting-group input:focus {
            outline: none;
            border-color: #667eea;
            box-shadow: 0 0 0 3px rgba(102, 126, 234, 0.1);
        }

        .toggle-btn {
            background: rgba(0, 0, 0, 0.05);
            color: #374151;
            border: 1px solid rgba(0, 0, 0, 0.12);
        }

        .toggle-btn:hover {
            background: rgba(0, 0, 0, 0.08);
            box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
        }

        .apply-btn {
            width: 100%;
            margin-top: 8px;
            padding: 12px;
            font-size: 14px;
        }

        /* Responsive adjustments */
        @media (max-width: 480px) {
            #unified-bot-ui {
                right: 12px;
                width: calc(100vw - 24px);
                max-width: 380px;
            }
        }

        /* Dark mode support */
        @media (prefers-color-scheme: dark) {
            #unified-bot-ui {
                background: rgba(31, 41, 55, 0.95);
                border-color: rgba(255, 255, 255, 0.1);
            }
            
            .section-title, .info-value {
                color: #f3f4f6;
            }
            
            .info-label {
                color: #9ca3af;
            }
            
            .chunk-preview {
                background: rgba(255, 255, 255, 0.05);
                color: #d1d5db;
            }
        }
    </style>
        <div id="unified-bot-container">
            <!-- Header -->
            <div id="bot-header">
                <div>
                    <h3 id="bot-title">Unified Bot System</h3>
                    <p id="bot-status">Ready</p>
                </div>
                <button id="close-bot-ui">×</button>
            </div>
            
            <!-- Auto-Continue Section -->
            <div class="section" id="auto-continue-section">
                <div class="section-header">
                    <h4 class="section-title">Auto-Continue</h4>
                    <div class="controls-group">
                        <button id="start-auto-btn" class="control-btn">▶ Start</button>
                        <button id="stop-auto-btn" class="control-btn accent">⏸ Stop</button>
                        <button id="manual-continue-btn" class="control-btn secondary">📤 Send</button>
                    </div>
                </div>
                <div class="info-grid">
                    <div class="info-card">
                        <div class="info-label">Status</div>
                        <div class="info-value">
                            <span class="status-indicator">
                                <span class="status-dot" id="status-dot"></span>
                                <span id="auto-status">Stopped</span>
                            </span>
                        </div>
                    </div>
                    <div class="info-card">
                        <div class="info-label">Interval</div>
                        <div class="info-value">${settings.autoContinue.intervalMinutes} min</div>
                    </div>
                    <div class="info-card">
                        <div class="info-label">End Condition</div>
                        <div class="info-value">${settings.autoContinue.endCondition}</div>
                    </div>
                    <div class="info-card">
                        <div class="info-label">Last Check</div>
                        <div class="info-value" id="last-check">Never</div>
                    </div>
                </div>
            </div>
            
            <!-- Text Extraction Section -->
            <div class="section" id="extraction-section">
                <div class="section-header">
                    <h4 class="section-title">Text Extraction</h4>
                    <div class="controls-group">
                        <button id="extract-btn" class="control-btn">🔍 Extract</button>
                        <button id="copy-all-btn" class="control-btn secondary">📋 Copy All</button>
                    </div>
                </div>
                <div class="info-grid">
                    <div class="info-card">
                        <div class="info-label">Chunks</div>
                        <div class="info-value" id="chunk-count">0</div>
                    </div>
                    <div class="info-card">
                        <div class="info-label">Total Characters</div>
                        <div class="info-value" id="total-chars">0</div>
                    </div>
                </div>
                
                <!-- Chunks Display -->
                <div id="chunks-display">
                    <div id="chunks-container"></div>
                </div>
            </div>
            
            <!-- Settings Section -->
            <div class="section" id="settings-section">
                <button id="toggle-settings-btn" class="control-btn toggle-btn">⚙️ Settings</button>
                <div id="settings-panel" class="settings-panel" style="display: none;">
                    <div class="setting-group">
                        <label>Continue Text:</label>
                        <input type="text" id="continue-text-input" value="${settings.autoContinue.continueText}">
                    </div>
                    <div class="setting-group">
                        <label>Interval (minutes):</label>
                        <input type="number" id="interval-input" value="${settings.autoContinue.intervalMinutes}" min="1" max="10">
                    </div>
                    <div class="setting-group">
                        <label>Max Chunk Size:</label>
                        <input type="number" id="chunk-size-input" value="${settings.extraction.maxChars}" min="1000" max="20000">
                    </div>
                    <button id="apply-settings-btn" class="control-btn apply-btn">Apply Settings</button>
                </div>
            </div>
        </div>
    `;
    }

    // Public API
    const publicAPI = {
        // Auto-continue functions
        startAutoSubmit,
        stopAutoSubmit,
        typeAndSubmit,
        checkForEndCondition,

        // Extraction functions
        performExtraction,
        copyAllChunks,
        getChunks: () => extractedChunks,
        getCleanedText: () => cleanedText,

        // UI functions
        showUI: createUI,
        hideUI: () => {
            const ui = document.getElementById('unified-bot-ui');
            if (ui) ui.remove();
        },
        updateUI,

        // Settings
        getSettings: () => settings,
        updateSettings: (newConfig) => {
            Object.assign(settings, newConfig);
            updateUI();
        },

        // Status
        getStatus: () => ({
            autoRunning: isAutoRunning,
            chunksCount: extractedChunks.length,
            totalChars: cleanedText.length,
            lastUpdate: new Date().toISOString()
        })
    };

    // Initialize
    createUI();
    log('Unified Bot System initialized', 'success');

    // Make functions globally available
    Object.assign(window, {
        unifiedBot: publicAPI,
        startAutoSubmit,
        stopAutoSubmit,
        performExtraction,
        copyAllChunks
    });

    return publicAPI;
}

// Initialize the system
const botSystem = createUnifiedBotSystem();

console.log('🤖 Unified Bot System loaded!');
console.log('📱 UI opened on the right side of your screen');
console.log('🔧 Available commands:');
console.log('  - unifiedBot.startAutoSubmit() - Start auto-continue');
console.log('  - unifiedBot.stopAutoSubmit() - Stop auto-continue');
console.log('  - unifiedBot.performExtraction() - Extract and chunk text');
console.log('  - unifiedBot.copyAllChunks() - Copy all chunks as JSON');
console.log('  - unifiedBot.getStatus() - Get current system status');