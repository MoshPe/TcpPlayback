import { useState, useEffect } from 'react';
import './App.css';
import {
  StartRecording,
  StopRecording,
  StartPlayback,
  StopPlayback,
  IsRecording,
  IsPlaying,
  GetRecordedFiles,
  GetServerAddress,
  GetCurrentSession,
  SetSessionName,
  SetPlaybackConfig,
  GetPlaybackConfig,
  GetLogEntries,
  ClearLogs,
  SetRecordingsPath,
  GetRecordingsPath,
  SelectFolder,
  GetFileInfo,
  SetRecordingConfig
} from "../wailsjs/go/main/App";

interface RecordedFile {
  path: string;
  session: string;
  fileName: string;
  size: number;
  modTime: string;
  magicWord: string;
  messagesPerFile: number;
  maxFileSize: number;
  messageCount: number;
  isOpen: boolean;
}

interface SessionGroup {
  sessionName: string;
  files: RecordedFile[];
}

interface LogEntry {
  time: string;
  fromIP: string;
  fromPort: number;
  toAddress: string;
  toPort: number;
  method: string;
  data: string;
  bytes: number;
}

function App() {
  const [isRecording, setIsRecording] = useState(false);
  const [isPlaying, setIsPlaying] = useState(false);
  const [recordPort, setRecordPort] = useState(8080);
  const [sessionName, setSessionName] = useState('');
  const [playHost, setPlayHost] = useState('localhost');
  const [playPort, setPlayPort] = useState(8080);
  const [selectedFile, setSelectedFile] = useState('');
  const [recordedFiles, setRecordedFiles] = useState<RecordedFile[]>([]);
  const [sessionGroups, setSessionGroups] = useState<SessionGroup[]>([]);
  const [serverAddress, setServerAddress] = useState('');
  const [currentSession, setCurrentSession] = useState('');
  const [status, setStatus] = useState('Ready to record and playback TCP data');

  // Playback configuration
  const [initDelay, setInitDelay] = useState(0);
  const [repeat, setRepeat] = useState(0);
  const [repeatDelay, setRepeatDelay] = useState(0);
  const [enableInitDelay, setEnableInitDelay] = useState(false);
  const [enableRepeat, setEnableRepeat] = useState(false);
  const [enableRepeatDelay, setEnableRepeatDelay] = useState(false);
  const [enableLoopOnEnd, setEnableLoopOnEnd] = useState(false);

  // Recording configuration
  const [magicWord, setMagicWord] = useState('');
  const [customMagicWord, setCustomMagicWord] = useState('');
  const [messagesPerFile, setMessagesPerFile] = useState(0);
  const [maxFileSize, setMaxFileSize] = useState(0);
  const [enableMagicWord, setEnableMagicWord] = useState(false);
  const [enableMessagesPerFile, setEnableMessagesPerFile] = useState(false);
  const [enableMaxFileSize, setEnableMaxFileSize] = useState(false);

  // Recordings path configuration
  const [recordingsPath, setRecordingsPath] = useState('recordings');
  const [recordingsPathError, setRecordingsPathError] = useState('');

  // Log table
  const [logEntries, setLogEntries] = useState<LogEntry[]>([]);
  
  // Tab state
  const [activeTab, setActiveTab] = useState('main');
  
  // File info modal
  const [showFileInfo, setShowFileInfo] = useState(false);
  const [selectedFileInfo, setSelectedFileInfo] = useState<RecordedFile | null>(null);

  // Status polling
  useEffect(() => {
    const pollStatus = () => {
      IsRecording().then(setIsRecording);
      IsPlaying().then((playing) => {
        // If playback was active and now stopped, show completion message
        if (isPlaying && !playing) {
          setStatus('Playback completed successfully!');
        }
        setIsPlaying(playing);
      });
      
      // Refresh logs periodically
      refreshLogs();
    };

    pollStatus();
    const interval = setInterval(pollStatus, 1000);
    return () => clearInterval(interval);
  }, []); // Removed isPlaying dependency to prevent frequent re-polling

  // Load playback configuration from backend on mount
  useEffect(() => {
    const loadPlaybackConfig = async () => {
      try {
        const config = await GetPlaybackConfig();
        console.log('Loading playback config from backend:', config);
        
        // Update frontend state with backend config
        setInitDelay(config.initDelay || 0);
        setRepeat(config.repeat || 0);
        setRepeatDelay(config.repeatDelay || 0);
        setEnableLoopOnEnd(config.loopOnEnd || false);
        
        // Set enable flags based on whether values are non-zero
        setEnableInitDelay(config.initDelay > 0);
        setEnableRepeat(config.repeat > 0);
        setEnableRepeatDelay(config.repeatDelay > 0);
        
        console.log('Playback config loaded successfully');
      } catch (error) {
        console.error('Error loading playback config:', error);
      }
    };

    loadPlaybackConfig();
  }, []); // Only run once on mount

  // Load recordings path from backend on mount
  useEffect(() => {
    const loadRecordingsPath = async () => {
      try {
        const path = await GetRecordingsPath();
        setRecordingsPath(path);
        setRecordingsPathError('');
      } catch (error) {
        console.error('Error loading recordings path:', error);
        setRecordingsPathError('Failed to load recordings path');
      }
    };

    loadRecordingsPath();
  }, []); // Only run once on mount

  // Update server info when recording starts
  useEffect(() => {
    if (isRecording) {
      GetServerAddress().then(setServerAddress);
      GetCurrentSession().then(setCurrentSession);
    }
  }, [isRecording]);

  const handleStartRecording = async () => {
    try {
      // Set session name if provided
      if (sessionName.trim()) {
        await SetSessionName(sessionName.trim());
      }
      
      // Apply recording configuration
      const finalMagicWord = magicWord === 'custom' ? customMagicWord : magicWord;
      console.log('Recording config before setting:', {
        magicWord: enableMagicWord ? finalMagicWord : '',
        messagesPerFile: enableMessagesPerFile ? messagesPerFile : 0,
        maxFileSize: enableMaxFileSize ? maxFileSize * 1024 * 1024 : 0 // Convert MB to bytes
      });

      // Set recording configuration
      await SetRecordingConfig(
        enableMagicWord ? finalMagicWord : '',
        enableMessagesPerFile ? messagesPerFile : 0,
        enableMaxFileSize ? maxFileSize * 1024 * 1024 : 0
      );
      
      console.log('Recording config set successfully');
      
      await StartRecording(recordPort);
      setStatus(`Recording started on port ${recordPort}${sessionName.trim() ? ` with session: ${sessionName.trim()}` : ''}`);
      await refreshRecordedFiles();
    } catch (error) {
      setStatus(`Recording error: ${error}`);
    }
  };

  const handleStopRecording = async () => {
    try {
      await StopRecording();
      setStatus('Recording stopped');
      await refreshRecordedFiles();
    } catch (error) {
      setStatus(`Stop recording error: ${error}`);
    }
  };

  const handleStartPlayback = async () => {
    if (!selectedFile) {
      setStatus('Please select a file to playback');
      return;
    }

    try {
      console.log('Playback config before setting:', {
        initDelay: enableInitDelay ? initDelay : 0,
        repeat: enableRepeat ? repeat : 0,
        repeatDelay: enableRepeatDelay ? repeatDelay : 0,
        loopOnEnd: enableLoopOnEnd
      });

      // Set playback configuration
      await SetPlaybackConfig(
        enableInitDelay ? initDelay : 0,
        enableRepeat ? repeat : 0,
        enableRepeatDelay ? repeatDelay : 0,
        enableLoopOnEnd
      );
      
      console.log('Playback config set successfully');
      
      // Refresh the configuration to ensure UI is in sync
      await refreshPlaybackConfig();
      
      // Get and log the current configuration to verify
      try {
        const currentConfig = await GetPlaybackConfig();
        console.log('Current playback config:', currentConfig);
      } catch (configError) {
        console.error('Error getting playback config:', configError);
      }
      
      await StartPlayback(selectedFile, playHost, playPort);
      setStatus(`Playback started to ${playHost}:${playPort} with configuration applied`);
    } catch (error) {
      console.error('Playback error:', error);
      setStatus(`Playback error: ${error}`);
    }
  };

  const handleStopPlayback = async () => {
    try {
      await StopPlayback();
      setStatus('Playback stopped');
    } catch (error) {
      setStatus(`Stop playback error: ${error}`);
    }
  };

  const refreshRecordedFiles = async () => {
    try {
      setStatus('Refreshing recorded files...');
      const files = await GetRecordedFiles();
      console.log('Backend returned files:', files);
      console.log('Number of files returned:', files.length);
      
      // Convert FileInfo to RecordedFile format
      const fileList: RecordedFile[] = files.map(fileInfo => ({
        path: fileInfo.path,
        session: fileInfo.session,
        fileName: fileInfo.fileName,
        size: fileInfo.size,
        modTime: fileInfo.modTime,
        magicWord: fileInfo.magicWord,
        messagesPerFile: fileInfo.messagesPerFile,
        maxFileSize: fileInfo.maxFileSize,
        messageCount: fileInfo.messageCount,
        isOpen: fileInfo.isOpen
      }));
      
      console.log('Converted file list:', fileList);
      console.log('Converted file list length:', fileList.length);
      
      // Clear the lists if no files found
      if (fileList.length === 0) {
        console.log('No files found, clearing lists');
        setRecordedFiles([]);
        setSessionGroups([]);
        setStatus('No recorded files found');
        return;
      }
      
      // Group files by session
      const sessionMap = new Map<string, RecordedFile[]>();
      fileList.forEach(file => {
        if (!sessionMap.has(file.session)) {
          sessionMap.set(file.session, []);
        }
        sessionMap.get(file.session)!.push(file);
      });
      
      // Convert to sorted array of session groups
      const groups: SessionGroup[] = Array.from(sessionMap.entries())
        .map(([sessionName, files]) => ({ sessionName, files }))
        .sort((a, b) => a.sessionName.localeCompare(b.sessionName));
      
      console.log('Session groups:', groups);
      
      setRecordedFiles(fileList);
      setSessionGroups(groups);
      setStatus(`Found ${fileList.length} recorded file(s) in ${groups.length} session(s)`);
    } catch (error) {
      console.error('Error refreshing files:', error);
      // Clear the lists on error as well
      setRecordedFiles([]);
      setSessionGroups([]);
      setStatus(`Error refreshing files: ${error}`);
    }
  };

  const refreshLogs = async () => {
    try {
      const logs = await GetLogEntries();
      setLogEntries(logs);
    } catch (error) {
      console.error('Error refreshing logs:', error);
    }
  };

  const refreshPlaybackConfig = async () => {
    try {
      const config = await GetPlaybackConfig();
      console.log('Refreshing playback config:', config);
      
      // Update frontend state with backend config
      setInitDelay(config.initDelay || 0);
      setRepeat(config.repeat || 0);
      setRepeatDelay(config.repeatDelay || 0);
      setEnableLoopOnEnd(config.loopOnEnd || false);
      
      // Set enable flags based on whether values are non-zero
      setEnableInitDelay(config.initDelay > 0);
      setEnableRepeat(config.repeat > 0);
      setEnableRepeatDelay(config.repeatDelay > 0);
    } catch (error) {
      console.error('Error refreshing playback config:', error);
    }
  };

  const handleClearLogs = async () => {
    try {
      await ClearLogs();
      setLogEntries([]);
      setStatus('Logs cleared successfully');
    } catch (error) {
      setStatus(`Error clearing logs: ${error}`);
    }
  };

  const handleSetRecordingsPath = async () => {
    try {
      setRecordingsPathError('');
      await SetRecordingsPath(recordingsPath);
      setStatus(`Recordings path set to: ${recordingsPath}`);
      await refreshRecordedFiles(); // Refresh files to show new path
    } catch (error) {
      setRecordingsPathError(`Failed to set path: ${error}`);
      setStatus(`Error setting recordings path: ${error}`);
    }
  };

  const handleBrowseFolder = async () => {
    try {
      const selectedPath = await SelectFolder();
      console.log('Selected folder:', selectedPath);
      // Handle both string and object return values
      let path = '';
      if (typeof selectedPath === 'string') {
        path = selectedPath;
      } else if (selectedPath && typeof selectedPath === 'object' && 'result' in selectedPath) {
        path = (selectedPath as { result: string }).result;
      }
      if (path) {
        setRecordingsPath(path);
        setRecordingsPathError('');
        setStatus(`Selected folder: ${path}`);
      }
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : String(error);
      if (errorMessage.includes('cancelled')) {
        setStatus('Folder selection cancelled');
      } else {
        setRecordingsPathError(`Failed to select folder: ${errorMessage}`);
        setStatus(`Error selecting folder: ${errorMessage}`);
      }
    }
  };

  const handleShowFileInfo = async (filePath: string) => {
    try {
      console.log('Getting fresh file info for:', filePath);
      const fileInfo = await GetFileInfo(filePath);
      console.log('Fresh file info received:', fileInfo);
      
      // Convert FileInfo to RecordedFile format
      const freshFileInfo: RecordedFile = {
        path: fileInfo.path,
        session: fileInfo.session,
        fileName: fileInfo.fileName,
        size: fileInfo.size,
        modTime: fileInfo.modTime,
        magicWord: fileInfo.magicWord,
        messagesPerFile: fileInfo.messagesPerFile,
        maxFileSize: fileInfo.maxFileSize,
        messageCount: fileInfo.messageCount,
        isOpen: fileInfo.isOpen
      };
      
      setSelectedFileInfo(freshFileInfo);
      setShowFileInfo(true);
    } catch (error) {
      console.error('Error getting file info:', error);
      setStatus(`Error getting file info: ${error}`);
    }
  };

  return (
    <div className="app">
      <header className="app-header">
        <h1>🔴 TCP Playback</h1>
        <p>Record and replay TCP data streams</p>
      </header>

      <div className="main-container">
        {/* Tabs */}
        <div className="tabs-container">
          <button 
            className={`tab-button ${activeTab === 'main' ? 'active' : ''}`}
            onClick={() => setActiveTab('main')}
          >
            🎛️ Main Controls
          </button>
          <button 
            className={`tab-button ${activeTab === 'files' ? 'active' : ''}`}
            onClick={() => setActiveTab('files')}
          >
            📁 Files
          </button>
          <button 
            className={`tab-button ${activeTab === 'logs' ? 'active' : ''}`}
            onClick={() => setActiveTab('logs')}
          >
            📊 Activity Log
          </button>
        </div>

        {/* Tab Content */}
        <div className="tab-content">
          {/* Main Controls Tab */}
          <div className={`tab-panel ${activeTab === 'main' ? 'active' : ''}`}>
            {/* Compact Status */}
            <div className="status-compact">
              <div className={`status-indicator-compact ${isRecording ? 'active' : ''}`}>
                <span className="status-dot-compact"></span>
                Recording: {isRecording ? 'Active' : 'Inactive'}
              </div>
              <div className={`status-indicator-compact ${isPlaying ? 'active' : ''}`}>
                <span className="status-dot-compact"></span>
                Playback: {isPlaying ? 'Active' : 'Inactive'}
              </div>
            </div>

            {/* Main Controls */}
            <div className="main-controls">
              {/* Recording Section */}
              <div className="control-section">
                <h3>📹 Recording</h3>
                <div className="control-group">
                  <div className="input-group">
                    <label>Port:</label>
                    <input
                      type="number"
                      value={recordPort}
                      onChange={(e) => {
                        console.log('Port change attempt:', e.target.value, 'isRecording:', isRecording);
                        const newPort = parseInt(e.target.value) || 8080;
                        console.log('Setting port to:', newPort);
                        setRecordPort(newPort);
                      }}
                      disabled={isRecording}
                      min="1"
                      max="65535"
                    />
                  </div>
                  
                  <div className="input-group">
                    <label>Session Name (optional):</label>
                    <input
                      type="text"
                      value={sessionName}
                      onChange={(e) => setSessionName(e.target.value)}
                      disabled={isRecording}
                      placeholder="e.g., test_session, production_data"
                    />
                  </div>
                  
                  {/* Recording Configuration */}
                  <div className="recording-config">
                    <h4>⚙️ Recording Configuration</h4>
                    
                    <div className="config-group">
                      <div className="config-item">
                        <label className="checkbox-label">
                          <input
                            type="checkbox"
                            checked={enableMagicWord}
                            onChange={(e) => setEnableMagicWord(e.target.checked)}
                            disabled={isRecording}
                          />
                          <span>Magic Word (message separator)</span>
                        </label>
                        <div className="magic-word-input">
                          <select
                            value={magicWord}
                            onChange={(e) => {
                              setMagicWord(e.target.value);
                              if (e.target.value !== 'custom') {
                                setCustomMagicWord('');
                              }
                            }}
                            disabled={isRecording || !enableMagicWord}
                            onFocus={() => {
                              if (magicWord === '') setMagicWord('\n');
                            }}
                          >
                            <option value="">Select separator...</option>
                            <option value="\n">New Line (\n)</option>
                            <option value="\r\n">Carriage Return + New Line (\r\n)</option>
                            <option value="\r">Carriage Return (\r)</option>
                            <option value="\t">Tab (\t)</option>
                            <option value=" ">Space</option>
                            <option value="|">Pipe (|)</option>
                            <option value=";">Semicolon (;)</option>
                            <option value=",">Comma (,)</option>
                            <option value="END">END</option>
                            <option value="custom">Custom...</option>
                          </select>
                          {magicWord === 'custom' && (
                            <input
                              type="text"
                              value={customMagicWord}
                              onChange={(e) => setCustomMagicWord(e.target.value)}
                              disabled={isRecording || !enableMagicWord}
                              placeholder="Enter custom separator..."
                              className="custom-magic-word"
                            />
                          )}
                        </div>
                      </div>

                      <div className="config-item">
                        <label className="checkbox-label">
                          <input
                            type="checkbox"
                            checked={enableMessagesPerFile}
                            onChange={(e) => setEnableMessagesPerFile(e.target.checked)}
                            disabled={isRecording}
                          />
                          <span>Messages Per File (0 = unlimited)</span>
                        </label>
                        <input
                          type="number"
                          value={messagesPerFile}
                          onChange={(e) => setMessagesPerFile(parseInt(e.target.value) || 0)}
                          disabled={isRecording || !enableMessagesPerFile}
                          min="0"
                          placeholder="100"
                        />
                      </div>

                      <div className="config-item">
                        <label className="checkbox-label">
                          <input
                            type="checkbox"
                            checked={enableMaxFileSize}
                            onChange={(e) => setEnableMaxFileSize(e.target.checked)}
                            disabled={isRecording}
                          />
                          <span>Max File Size (MB)</span>
                        </label>
                        <input
                          type="number"
                          value={maxFileSize}
                          onChange={(e) => setMaxFileSize(parseInt(e.target.value) || 0)}
                          disabled={isRecording || !enableMaxFileSize}
                          min="0"
                          placeholder="10"
                        />
                      </div>
                    </div>
                  </div>
                  
                  <div className="button-group">
                    {!isRecording ? (
                      <button 
                        className="btn btn-record"
                        onClick={handleStartRecording}
                      >
                        🎙️ Start Recording
                      </button>
                    ) : (
                      <button 
                        className="btn btn-stop"
                        onClick={handleStopRecording}
                      >
                        ⏹️ Stop Recording
                      </button>
                    )}
                  </div>
                </div>

                {isRecording && (
                  <div className="status-info">
                    <p><strong>Server Address:</strong> {serverAddress}</p>
                    <p><strong>Session:</strong> {currentSession}</p>
                  </div>
                )}
              </div>

              {/* Playback Section */}
              <div className="control-section">
                <h3>▶️ Playback</h3>
                <div className="control-group">
                  <div className="input-group">
                    <label>Host:</label>
                    <input
                      type="text"
                      value={playHost}
                      onChange={(e) => setPlayHost(e.target.value)}
                      disabled={isPlaying}
                      placeholder="localhost"
                    />
                  </div>
                  
                  <div className="input-group">
                    <label>Port:</label>
                    <input
                      type="number"
                      value={playPort}
                      onChange={(e) => setPlayPort(parseInt(e.target.value) || 8080)}
                      disabled={isPlaying}
                      min="1"
                      max="65535"
                    />
                  </div>
                </div>

                <div className="file-selection">
                  <label>Select File:</label>
                  <select
                    value={selectedFile}
                    onChange={(e) => setSelectedFile(e.target.value)}
                    disabled={isPlaying || recordedFiles.length === 0}
                  >
                    <option value="">Choose a recorded file...</option>
                    {recordedFiles.map((file, index) => (
                      <option key={index} value={file.path}>
                        {file.fileName}
                      </option>
                    ))}
                  </select>
                </div>

                {/* Playback Configuration */}
                <div className="playback-config">
                  <h4>⚙️ Configuration</h4>
                  
                  <div className="config-group">
                    <div className="config-item">
                      <label className="checkbox-label">
                        <input
                          type="checkbox"
                          checked={enableInitDelay}
                          onChange={(e) => setEnableInitDelay(e.target.checked)}
                          disabled={isPlaying}
                        />
                        <span>Initial Delay (ms)</span>
                      </label>
                      <input
                        type="number"
                        value={initDelay}
                        onChange={(e) => setInitDelay(parseInt(e.target.value) || 0)}
                        disabled={isPlaying || !enableInitDelay}
                        min="0"
                        placeholder="1000"
                      />
                    </div>

                    <div className="config-item">
                      <label className="checkbox-label">
                        <input
                          type="checkbox"
                          checked={enableRepeat}
                          onChange={(e) => setEnableRepeat(e.target.checked)}
                          disabled={isPlaying}
                        />
                        <span>Repeat Count (0 = infinite)</span>
                      </label>
                      <input
                        type="number"
                        value={repeat}
                        onChange={(e) => setRepeat(parseInt(e.target.value) || 0)}
                        disabled={isPlaying || !enableRepeat}
                        min="0"
                        placeholder="5"
                      />
                    </div>

                    <div className="config-item">
                      <label className="checkbox-label">
                        <input
                          type="checkbox"
                          checked={enableRepeatDelay}
                          onChange={(e) => setEnableRepeatDelay(e.target.checked)}
                          disabled={isPlaying}
                        />
                        <span>Repeat Delay (ms)</span>
                      </label>
                      <input
                        type="number"
                        value={repeatDelay}
                        onChange={(e) => setRepeatDelay(parseInt(e.target.value) || 0)}
                        disabled={isPlaying || !enableRepeatDelay}
                        min="0"
                        placeholder="2000"
                      />
                    </div>

                    <div className="config-item">
                      <label className="checkbox-label">
                        <input
                          type="checkbox"
                          checked={enableLoopOnEnd}
                          onChange={(e) => setEnableLoopOnEnd(e.target.checked)}
                          disabled={isPlaying}
                        />
                        <span>Loop on End</span>
                      </label>
                    </div>
                  </div>
                </div>

                <div className="button-group">
                  {!isPlaying ? (
                    <button 
                      className="btn btn-play"
                      onClick={handleStartPlayback}
                      disabled={!selectedFile}
                    >
                      ▶️ Start Playback
                    </button>
                  ) : (
                    <button 
                      className="btn btn-stop"
                      onClick={handleStopPlayback}
                    >
                      ⏹️ Stop Playback
                    </button>
                  )}
                  <button 
                    className="btn btn-test"
                    onClick={async () => {
                      try {
                        await SetPlaybackConfig(1000, 3, 2000, true);
                        const config = await GetPlaybackConfig();
                        console.log('Test config set:', config);
                        setStatus('Test configuration applied successfully');
                      } catch (error) {
                        console.error('Test config error:', error);
                        setStatus(`Test config error: ${error}`);
                      }
                    }}
                  >
                    🧪 Test Config
                  </button>
                  <button 
                    className="btn btn-refresh"
                    onClick={async () => {
                      try {
                        await refreshPlaybackConfig();
                        setStatus('Playback configuration refreshed');
                      } catch (error) {
                        console.error('Refresh config error:', error);
                        setStatus(`Refresh config error: ${error}`);
                      }
                    }}
                  >
                    🔄 Refresh Config
                  </button>
                </div>
              </div>
            </div>

            {/* Status Message */}
            <div className="status-message">
              {status}
            </div>
          </div>

          {/* Files Tab */}
          <div className={`tab-panel ${activeTab === 'files' ? 'active' : ''}`}>
            {/* Recordings Path Configuration */}
            <div className="recordings-path-config">
              <h3>📂 Recordings Directory</h3>
              <div className="path-config-group">
                <div className="input-group">
                  <label>Base Path:</label>
                  <input
                    type="text"
                    value={recordingsPath}
                    onChange={(e) => setRecordingsPath(e.target.value)}
                    placeholder="e.g., recordings, C:\MyRecordings, /home/user/recordings"
                    className={recordingsPathError ? 'error' : ''}
                  />
                </div>
                <button 
                  className="btn btn-secondary"
                  onClick={handleBrowseFolder}
                >
                  📁 Browse
                </button>
                <button 
                  className="btn btn-primary"
                  onClick={handleSetRecordingsPath}
                >
                  🔧 Set Path
                </button>
              </div>
              {recordingsPathError && (
                <div className="error-message">
                  {recordingsPathError}
                </div>
              )}
            </div>

            <div className="files-header">
              <h2>📁 Recorded Files</h2>
              <div className="files-stats">
                <span>{recordedFiles.length} Total Files</span>
                <span>{sessionGroups.length} Sessions</span>
                <button 
                  className="btn btn-refresh"
                  onClick={refreshRecordedFiles}
                >
                  🔄 Refresh
                </button>
              </div>
            </div>
            
            {recordedFiles.length === 0 ? (
              <p className="no-files">No recorded files yet. Start recording to capture TCP data.</p>
            ) : (
              <div className="sessions-list">
                {sessionGroups.map((group, groupIndex) => (
                  <div key={groupIndex} className="session-group">
                    <div className="session-header">
                      <span className="session-name">{group.sessionName}</span>
                      <span className="session-count">{group.files.length} file(s)</span>
                    </div>
                    <div className="files-list">
                      {group.files.map((file, fileIndex) => (
                        <div 
                          key={fileIndex} 
                          className={`file-item ${selectedFile === file.path ? 'selected' : ''}`}
                          onClick={() => setSelectedFile(file.path)}
                        >
                          <div className="file-info">
                            <span className="file-name">{file.fileName}</span>
                            <div className="file-details">
                              <span className="file-size">Session: {file.session}</span>
                              <span>Size: {file.size ? `${(file.size / 1024).toFixed(1)} KB` : 'Unknown'}</span>
                              {file.messageCount > 0 && (
                                <span>Messages: {file.messageCount}</span>
                              )}
                              {file.magicWord !== undefined && (
                                <span>Magic: "{file.magicWord ? file.magicWord.replace(/\r/g, '\\r').replace(/\n/g, '\\n').replace(/\t/g, '\\t') : 'None'}"</span>
                              )}
                            </div>
                          </div>
                          <div className="file-actions">
                            <button 
                              className="file-action-btn play"
                              onClick={(e) => {
                                e.stopPropagation();
                                setSelectedFile(file.path);
                                setActiveTab('main');
                              }}
                              disabled={file.isOpen}
                            >
                              ▶️ Play
                            </button>
                            <button 
                              className="file-action-btn"
                              onClick={(e) => {
                                e.stopPropagation();
                                handleShowFileInfo(file.path);
                              }}
                            >
                              ℹ️ Info
                            </button>
                          </div>
                        </div>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>

          {/* Logs Tab */}
          <div className={`tab-panel ${activeTab === 'logs' ? 'active' : ''}`}>
            <h2>📊 Activity Log</h2>
            <div className="log-controls">
              <button 
                className="btn btn-refresh"
                onClick={refreshLogs}
              >
                🔄 Refresh Logs
              </button>
              <button 
                className="btn btn-clear"
                onClick={handleClearLogs}
              >
                🗑️ Clear Logs
              </button>
            </div>
            
            <div className="log-table-container">
              <table className="log-table">
                <thead>
                  <tr>
                    <th>Time</th>
                    <th>From IP</th>
                    <th>From Port</th>
                    <th>To Address</th>
                    <th>To Port</th>
                    <th>Method</th>
                    <th>Data</th>
                    <th>Bytes</th>
                  </tr>
                </thead>
                <tbody>
                  {logEntries.length === 0 ? (
                    <tr>
                      <td colSpan={8} className="no-logs">No activity logged yet</td>
                    </tr>
                  ) : (
                    logEntries.map((entry, index) => (
                      <tr key={index} className={`log-row log-${entry.method.toLowerCase()}`}>
                        <td>{new Date(entry.time).toLocaleTimeString()}</td>
                        <td>{entry.fromIP}</td>
                        <td>{entry.fromPort || '-'}</td>
                        <td>{entry.toAddress}</td>
                        <td>{entry.toPort || '-'}</td>
                        <td>
                          <span className={`method-badge method-${entry.method.toLowerCase()}`}>
                            {entry.method}
                          </span>
                        </td>
                        <td>{entry.data}</td>
                        <td>{entry.bytes > 0 ? entry.bytes.toLocaleString() : '-'}</td>
                      </tr>
                    ))
                  )}
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </div>

      {/* File Info Modal */}
      {showFileInfo && selectedFileInfo && (
        <div 
          className="file-info-modal"
          onClick={() => setShowFileInfo(false)}
        >
          <div 
            className="file-info-content"
            onClick={(e) => e.stopPropagation()}
          >
            <h2>📄 File Information</h2>
            <div className="file-info-details">
              <p><strong>📁 File Name:</strong> {selectedFileInfo.fileName}</p>
              <p><strong>📂 Session:</strong> {selectedFileInfo.session}</p>
              <p><strong>📍 Full Path:</strong> {selectedFileInfo.path}</p>
              <p><strong>📊 File Type:</strong> Binary (.bin)</p>
              <p><strong>🎯 Status:</strong> {selectedFileInfo.isOpen ? 'Recording in progress' : 'Ready for playback'}</p>
              <p><strong>📏 File Size:</strong> {selectedFileInfo.size ? `${(selectedFileInfo.size / 1024).toFixed(2)} KB` : 'Unknown'}</p>
              <p><strong>📅 Modified:</strong> {selectedFileInfo.modTime}</p>
              {selectedFileInfo.magicWord !== undefined && (
                <p><strong>🔮 Magic Word:</strong> "{selectedFileInfo.magicWord ? selectedFileInfo.magicWord.replace(/\r/g, '\\r').replace(/\n/g, '\\n').replace(/\t/g, '\\t') : 'None'}"</p>
              )}
              {selectedFileInfo.messageCount > 0 && (
                <p><strong>💬 Messages:</strong> {selectedFileInfo.messageCount.toLocaleString()}</p>
              )}
              <p><strong>📝 Messages Per File:</strong> {selectedFileInfo.isOpen && !selectedFileInfo.messagesPerFile ? 'Unknown' : (selectedFileInfo.messagesPerFile > 0 ? selectedFileInfo.messagesPerFile : 'Unlimited')}</p>
              <p><strong>📏 Max File Size:</strong> {selectedFileInfo.isOpen && !selectedFileInfo.maxFileSize ? 'Unknown' : (selectedFileInfo.maxFileSize > 0 ? `${selectedFileInfo.maxFileSize} bytes` : 'Unlimited')}</p>
            </div>
            <div className="file-info-actions">
              <button 
                className="btn btn-play"
                onClick={() => {
                  setSelectedFile(selectedFileInfo.path);
                  setShowFileInfo(false);
                  setActiveTab('main');
                }}
                disabled={selectedFileInfo.isOpen}
              >
                ▶️ Play This File
              </button>
              <button 
                className="btn btn-close"
                onClick={() => setShowFileInfo(false)}
              >
                ✕ Close
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

export default App;
