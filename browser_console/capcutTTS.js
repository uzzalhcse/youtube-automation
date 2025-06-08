// Text Chunk Auto-Submitter Script
(function() {
    'use strict';

    let chunks = [];
    let currentChunkIndex = 0;
    let isProcessing = false;
    let intervalId = null;

    // Show notification in UI instead of alert
    function showNotification(message, type = 'info') {
        const notification = document.createElement('div');
        notification.style.cssText = `
            position: fixed;
            top: 20px;
            right: 20px;
            padding: 12px 20px;
            border-radius: 6px;
            color: white;
            font-weight: bold;
            z-index: 10001;
            max-width: 300px;
            word-wrap: break-word;
            transition: opacity 0.3s ease;
        `;

        // Set background color based on type
        switch(type) {
            case 'error':
                notification.style.background = '#dc3545';
                break;
            case 'warning':
                notification.style.background = '#ffc107';
                notification.style.color = '#000';
                break;
            case 'success':
                notification.style.background = '#28a745';
                break;
            default:
                notification.style.background = '#007bff';
        }

        notification.textContent = message;
        document.body.appendChild(notification);

        // Auto-remove after 4 seconds
        setTimeout(() => {
            notification.style.opacity = '0';
            setTimeout(() => {
                if (notification.parentNode) {
                    notification.parentNode.removeChild(notification);
                }
            }, 300);
        }, 4000);
    }

    // Create the UI
    function createUI() {
        // Remove existing UI if present
        const existingUI = document.getElementById('chunk-submitter-ui');
        if (existingUI) {
            existingUI.remove();
        }

        const ui = document.createElement('div');
        ui.id = 'chunk-submitter-ui';
        ui.innerHTML = `
            <div style="
                position: fixed;
                bottom: 20px;
                left: 20px;
                width: 400px;
                max-height: 600px;
                background: white;
                border: 2px solid #333;
                border-radius: 10px;
                box-shadow: 0 4px 20px rgba(0,0,0,0.3);
                z-index: 10000;
                font-family: Arial, sans-serif;
                font-size: 14px;
            ">
                <div style="
                    background: #333;
                    color: white;
                    padding: 10px;
                    border-radius: 8px 8px 0 0;
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                ">
                    <h3 style="margin: 0; font-size: 16px;">Text Chunk Submitter</h3>
                    <button id="close-ui" style="
                        background: none;
                        border: none;
                        color: white;
                        font-size: 18px;
                        cursor: pointer;
                        padding: 0;
                        width: 20px;
                        height: 20px;
                    " title="Hide UI">×</button>
                </div>
                
                <div style="padding: 15px;">
                    <div style="margin-bottom: 15px;">
                        <label style="display: block; margin-bottom: 5px; font-weight: bold;">Paste your long text here:</label>
                        <textarea id="input-text" style="
                            width: 100%;
                            height: 120px;
                            padding: 8px;
                            border: 1px solid #ccc;
                            border-radius: 4px;
                            resize: vertical;
                            font-size: 12px;
                        " placeholder="Paste your long story here..."></textarea>
                    </div>
                    
                    <div style="margin-bottom: 15px; display: flex; gap: 10px;">
                        <div style="flex: 1;">
                            <label style="display: block; margin-bottom: 5px; font-weight: bold; font-size: 12px;">Max Characters:</label>
                            <input id="max-chars" type="number" value="9050" min="100" max="10000" style="
                                width: 100%;
                                padding: 6px;
                                border: 1px solid #ccc;
                                border-radius: 4px;
                                font-size: 12px;
                            ">
                        </div>
                        <div style="flex: 1;">
                            <label style="display: block; margin-bottom: 5px; font-weight: bold; font-size: 12px;">Interval (seconds):</label>
                            <input id="interval-seconds" type="number" value="15" min="5" max="3600" style="
                                width: 100%;
                                padding: 6px;
                                border: 1px solid #ccc;
                                border-radius: 4px;
                                font-size: 12px;
                            ">
                        </div>
                    </div>
                    
                    <div style="margin-bottom: 15px;">
                        <button id="split-text" style="
                            background: #007bff;
                            color: white;
                            border: none;
                            padding: 8px 16px;
                            border-radius: 4px;
                            cursor: pointer;
                            margin-right: 10px;
                        ">Split Text</button>
                        
                        <button id="start-processing" style="
                            background: #28a745;
                            color: white;
                            border: none;
                            padding: 8px 16px;
                            border-radius: 4px;
                            cursor: pointer;
                            margin-right: 10px;
                        " disabled>Start Auto-Submit</button>
                        
                        <button id="stop-processing" style="
                            background: #dc3545;
                            color: white;
                            border: none;
                            padding: 8px 16px;
                            border-radius: 4px;
                            cursor: pointer;
                        " disabled>Stop</button>
                    </div>
                    
                    <div style="margin-bottom: 10px;">
                        <strong>Status: </strong><span id="status">Ready</span>
                    </div>
                    
                    <div style="
                        max-height: 200px;
                        overflow-y: auto;
                        border: 1px solid #ccc;
                        border-radius: 4px;
                        padding: 5px;
                        background: #f9f9f9;
                    ">
                        <div id="chunks-list" style="font-size: 12px;"></div>
                    </div>
                </div>
            </div>
        `;

        document.body.appendChild(ui);

        // Add event listeners
        document.getElementById('close-ui').addEventListener('click', () => {
            const uiContainer = ui.querySelector('div').children[1]; // The content container
            if (uiContainer.style.display === 'none') {
                uiContainer.style.display = 'block';
                document.getElementById('close-ui').textContent = '×';
                document.getElementById('close-ui').title = 'Hide UI';
            } else {
                uiContainer.style.display = 'none';
                document.getElementById('close-ui').textContent = '□';
                document.getElementById('close-ui').title = 'Show UI';
            }
        });

        document.getElementById('split-text').addEventListener('click', splitText);
        document.getElementById('start-processing').addEventListener('click', startProcessing);
        document.getElementById('stop-processing').addEventListener('click', stopProcessing);
    }

    // Split text into chunks
    function splitText() {
        const inputText = document.getElementById('input-text').value.trim();
        const maxChars = parseInt(document.getElementById('max-chars').value);

        if (!inputText) {
            showNotification('Please paste some text first!', 'warning');
            return;
        }

        if (!maxChars || maxChars < 100 || maxChars > 10000) {
            showNotification('Please enter a valid character limit (100-10,000)!', 'warning');
            return;
        }

        chunks = [];
        currentChunkIndex = 0;

        const sentences = inputText.split(/(?<=[.!?])\s+/);
        let currentChunk = '';

        for (let sentence of sentences) {
            // If adding this sentence would exceed max characters, save current chunk
            if (currentChunk.length + sentence.length + 1 > maxChars && currentChunk.length > 0) {
                chunks.push(currentChunk.trim());
                currentChunk = sentence;
            } else {
                currentChunk += (currentChunk ? ' ' : '') + sentence;
            }
        }

        // Add the last chunk if it has content
        if (currentChunk.trim()) {
            chunks.push(currentChunk.trim());
        }

        displayChunks();
        document.getElementById('start-processing').disabled = false;
        updateStatus(`Split into ${chunks.length} chunks (max ${maxChars} chars each)`);
        showNotification(`Successfully split text into ${chunks.length} chunks!`, 'success');
    }

    // Display chunks in the UI
    function displayChunks() {
        const chunksList = document.getElementById('chunks-list');
        chunksList.innerHTML = '';

        chunks.forEach((chunk, index) => {
            const chunkDiv = document.createElement('div');
            chunkDiv.style.cssText = `
                margin-bottom: 8px;
                padding: 8px;
                border: 1px solid #ddd;
                border-radius: 4px;
                background: white;
            `;

            const status = index < currentChunkIndex ? '✅ Completed' :
                index === currentChunkIndex && isProcessing ? '⏳ Processing' : '⏸️ Pending';

            chunkDiv.innerHTML = `
                <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 5px;">
                    <strong>Chunk ${index + 1}</strong>
                    <span style="font-size: 11px; color: ${index < currentChunkIndex ? 'green' : index === currentChunkIndex && isProcessing ? 'orange' : 'gray'};">
                        ${status}
                    </span>
                </div>
                <div style="font-size: 11px; color: #666; max-height: 40px; overflow: hidden;">
                    ${chunk.substring(0, 100)}${chunk.length > 100 ? '...' : ''}
                </div>
                <div style="font-size: 10px; color: #999; margin-top: 3px;">
                    ${chunk.length} characters
                </div>
            `;

            chunksList.appendChild(chunkDiv);
        });
    }

    // Start auto-processing
    function startProcessing() {
        if (chunks.length === 0) {
            showNotification('Please split text first!', 'warning');
            return;
        }

        const intervalSeconds = parseInt(document.getElementById('interval-seconds').value);

        if (!intervalSeconds || intervalSeconds < 5 || intervalSeconds > 3600) {
            showNotification('Please enter a valid interval (5-3600 seconds)!', 'warning');
            return;
        }

        isProcessing = true;
        document.getElementById('start-processing').disabled = true;
        document.getElementById('stop-processing').disabled = false;

        // Submit the first chunk immediately
        submitCurrentChunk();

        // Set interval based on user input (in milliseconds)
        const intervalMs = intervalSeconds * 1000;
        intervalId = setInterval(() => {
            if (currentChunkIndex < chunks.length) {
                submitCurrentChunk();
            } else {
                stopProcessing();
                updateStatus('All chunks completed!');
                showNotification('All chunks have been submitted successfully!', 'success');
            }
        }, intervalMs);

        updateStatus(`Auto-submission started (${intervalSeconds} sec intervals)`);
        showNotification(`Auto-submission started with ${intervalSeconds} second intervals`, 'info');
    }

    // Stop processing
    function stopProcessing() {
        isProcessing = false;
        if (intervalId) {
            clearInterval(intervalId);
            intervalId = null;
        }

        document.getElementById('start-processing').disabled = false;
        document.getElementById('stop-processing').disabled = true;
        updateStatus('Stopped');
        displayChunks();
    }

    function clearByTyping(element) {
        if (!element) return false;

        console.log('Attempting character-by-character clearing...');
        element.focus();

        // Get current text length
        const textLength = element.value ? element.value.length :
            element.textContent ? element.textContent.length : 0;

        // if (textLength === 0) return true;

        setTimeout(() => {
            const textLength = element.value ? element.value.length :
                element.textContent ? element.textContent.length : 0;

            // if (textLength === 0) return true;

            console.log("textLength:", textLength);
            const selectAllEvent = new KeyboardEvent('keydown', {
                key: 'a',
                code: 'KeyA',
                keyCode: 65,
                which: 65,
                ctrlKey: true,
                bubbles: true,
                cancelable: true
            });
            element.dispatchEvent(selectAllEvent);
            // Simulate backspace for each character

            setTimeout(() => {
                // Simulate backspace key
                const backspaceEvent = new KeyboardEvent('keydown', {
                    key: 'Backspace',
                    code: 'Backspace',
                    keyCode: 8,
                    which: 8,
                    bubbles: true,
                    cancelable: true
                });
                element.dispatchEvent(backspaceEvent);

                // Also dispatch input event
                const inputEvent = new Event('input', { bubbles: true });
                element.dispatchEvent(inputEvent);

            }, 100); // Small delay between each character deletion

            const textLength1 = element.value ? element.value.length :
                element.textContent ? element.textContent.length : 0;

            // if (textLength === 0) return true;

            console.log("textLength1:", textLength1);
        }, 100); // Small delay between each character deletion

        return true;
    }
    function clearTextFieldAggressive(element) {
        if (!element) return false;

        console.log('Starting aggressive text clearing sequence...');

        // Try methods in sequence with delays
        const methods = [
            { fn: clearByTyping, delay: 100 },
        ];

        methods.forEach(({ fn, delay }) => {
            setTimeout(() => {
                try {
                    fn(element);
                    console.log(`${fn.name} attempted`);
                } catch (error) {
                    console.warn(`${fn.name} failed:`, error);
                }
            }, delay);
        });


        return true;
    }
    // Safe text setting without illegal invocation errors
    function setTextInstantly(element, text) {
        if (!element) return false;

        try {
            element.focus();

            // Clear first, then set
            element.value = '';
            element.value = text;

            // For contentEditable elements
            if (element.contentEditable === 'true') {
                element.innerHTML = '';
                element.textContent = text;
            }

            // Trigger events for React/SPA compatibility
            const events = ['input', 'change', 'keyup'];
            events.forEach(eventType => {
                try {
                    const event = new Event(eventType, { bubbles: true, cancelable: true });
                    element.dispatchEvent(event);
                } catch (e) {
                    // Ignore individual event errors
                }
            });

            return true;
        } catch (error) {
            console.warn('Text setting failed:', error);
            return false;
        }
    }

    // Updated submit function
    function submitCurrentChunk() {
        if (currentChunkIndex >= chunks.length) {
            stopProcessing();
            return;
        }

        const chunk = chunks[currentChunkIndex];
        const editorContainer = document.querySelector('.editor-kit-container');
        const submitButton = document.querySelector('#tts-generate-btn button');

        if (!editorContainer) {
            console.error('Editor container not found!');
            updateStatus('Error: Editor not found');
            showNotification('Error: Editor container not found!', 'error');
            return;
        }

        if (!submitButton) {
            console.error('Submit button not found!');
            updateStatus('Error: Submit button not found');
            showNotification('Error: Submit button not found!', 'error');
            return;
        }

        try {
            // Clear existing content first
            // clearTextFieldAggressive(editorContainer);

            // Wait for clearing, then set new text
            setTimeout(() => {
                const success = setTextInstantly(editorContainer, chunk);

                if (!success) {
                    console.warn('Text setting failed, trying alternative...');
                    // Alternative approach for complex editors
                    editorContainer.focus();
                    editorContainer.innerHTML = '';
                    editorContainer.textContent = chunk;

                    const inputEvent = new Event('input', { bubbles: true });
                    editorContainer.dispatchEvent(inputEvent);
                }

                // Check button state after setting text
                setTimeout(() => {
                    const isButtonDisabled = submitButton.hasAttribute('disabled') ||
                        submitButton.getAttribute('data-disabled') === '1' ||
                        submitButton.classList.contains('lv-btn-disabled');

                    if (isButtonDisabled) {
                        console.warn('Generate button is disabled');
                        updateStatus('Warning: Generate button is disabled!');
                        showNotification('Generate button is disabled! Check requirements.', 'warning');

                        clearTextFieldAggressive(editorContainer);
                        stopProcessing();
                        return;
                    }

                    // Submit the chunk
                    submitButton.click();
                    console.log(`Submitted chunk ${currentChunkIndex + 1}/${chunks.length}`);

                    // Clear after submission
                    setTimeout(() => {
                        clearTextFieldAggressive(editorContainer);
                        console.log('Cleared text after submission');
                    }, 1000);

                    currentChunkIndex++;
                    updateStatus(`Submitted chunk ${currentChunkIndex}/${chunks.length}`);
                    displayChunks();

                }, 1500);
            }, 200);

        } catch (error) {
            console.error('Error in submitCurrentChunk:', error);
            updateStatus('Error submitting chunk');
            showNotification('Error occurred while submitting!', 'error');
        }
    }

    // Update status
    function updateStatus(message) {
        document.getElementById('status').textContent = message;
    }

    // Initialize the UI
    createUI();

    console.log('Text Chunk Submitter loaded! UI added to bottom-left corner.');

})();