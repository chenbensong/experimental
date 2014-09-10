import MySQLdb
import datetime
import glob
import httplib2
import os
import pickle
import re
import socket
import stat
import struct
import subprocess
import time

from buildbot.interfaces import BuildSlaveTooOldError
from buildbot.process import buildstep
from buildbot.status.results import SUCCESS, WARNINGS, FAILURE

_METADATA_URL = 'http://metadata/computeMetadata/v1/instance/attributes/'
_DB_HOST = '173.194.240.40'
_DB_USER = 'readwrite'
_DB_NAME = 'skia'
_SUB_RE = re.compile('[A-Z]+[0-9a-z]*[A-Z_\.]')

def _sanitizeGraphiteNames(string):
  return string.replace('.', '_')

def _extractSub(string):
  if string.startswith('/mnt'):
    return ''
  if string.find('.') < 0:
    return ''
  ret = string[string.find('.') + 1 : ]
  sub = _SUB_RE.match(ret[2:])
  if not sub or not sub.group(0):
    return ret[:ret.find('.')]

  return ret[:2] + sub.group(0)[:-1] 
    

class ProcessSize(buildstep.LoggingBuildStep):
  def __init__(self, workdir, **kwargs):
    buildstep.LoggingBuildStep.__init__(self, **kwargs)
    self.workdir = workdir
    self.log_str = 'Start Size.\n'

  def _commit(self, db, sql):
    cur = db.cursor()
    if cur.execute(sql) < 0:
      cur.close()
      self.log_str += 'DB INSERT Error: %s.\n' % (sql + ','.join(subvalues))
      self.addCompleteLog('nodbinsert', self.log_str)
      self.step_status.setText('Failed to insert new values.')
      self.finished(FAILURE)
      return
    try:
      db.commit()
    except Exception:
      db.rollback()
      self.log_str += 'DB Commit Error.\n'
      self.addCompleteLog('nodbcommit', self.log_str)
      self.step_status.setText('Failed commit inserts.')
      cur.close()
    cur.close()

  def start(self):
    ts = int(self.getProperty('ts'))
    values = []
    subvalues = []
    subtype = ['text', 'data', 'bss', 'dec']

    file_prefix = os.path.join(
        self.getProperty('builddir'), self.workdir, 'out', 'Release')
    files = [file_prefix + '/skia.so', file_prefix + '/lib/libskia.so']
    for f in glob.iglob(os.path.join(file_prefix, 'libskia*.a')):
        files.append(f)
    for f in files:
      name = _sanitizeGraphiteNames(os.path.basename(f))
      sizes = {}
      subs = {}
      proc = subprocess.Popen(['/usr/bin/size', f], stdout=subprocess.PIPE)
      out, err = proc.communicate()
      if not err and out:
        lines = out.split('\n')[1:]
        for l in lines:
          cols = l.strip().split('\t')
          if len(cols) != 6:
            continue
          o = cols[-1]
          sub = _extractSub(o)
          if not sub:
            self.log_str += 'Invalid sub %s\n' % o
            sub = 'UNKNOWN'
          if sub not in subs:
            subs[sub] = {}
          for i in range(len(subtype)):
            if subtype[i] not in sizes:
              sizes[subtype[i]] = 0
            if subtype[i] not in subs[sub]:
              subs[sub][subtype[i]] = 0
            sizes[subtype[i]] += int(cols[i])
            subs[sub][subtype[i]] += int(cols[i])
      ts_str = datetime.datetime.utcfromtimestamp(ts).strftime(
          '%Y-%m-%d %H:%M:%S')
      for key in sizes:
        values.append("('%s','%s','%s',%d)" % (ts_str, name, key, sizes[key]))
        self.log_str += values[-1] + '\n'
      for sub in subs:
        for t in subs[sub]:
          subvalues.append("('%s','%s','%s','%s','%d')" % (
              ts_str, name, sub, t, subs[sub][t]))

    if not len(subvalues):
      self.log_str += 'No results found.\n'
      self.addCompleteLog('noresult', self.log_str)
      self.step_status.setText('No libskia after build.')
      self.finished(WARNING)

    self.log_str += 'Try write to DB:\n%s\n' % '\n'.join(subvalues)
    http = httplib2.Http()
    resp, passwd = http.request(uri=_METADATA_URL+'readwrite', method='GET',
        headers={'Metadata-Flavor': 'Google'})
    if resp == 500:
      self.log_str += 'Cannot get DB PASS.\n'
      self.addCompleteLog('nodbpass', self.log_str)
      self.step_status.setText('Failed to read Cloud SQL password.')
      self.finished(FAILURE)
    db = MySQLdb.connect(host=_DB_HOST, user=_DB_USER, passwd=passwd,
        db=_DB_NAME)
    sql = 'INSERT IGNORE INTO size(ts,file,type,size) VALUES'
    self._commit(db, sql + ','.join(values))
    sql = 'INSERT IGNORE INTO subsize(ts,file,sub,type,size) VALUES'
    self._commit(db, sql + ','.join(subvalues))

    db.close()

    self.log_str += 'Done.\n'
    self.addCompleteLog('done', self.log_str)
    self.step_status.setText(
        'Processed %d Records.' % len(subvalues))
    self.finished(SUCCESS)

