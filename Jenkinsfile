pipeline {
    agent {
      kubernetes {
        cloud 'kubernetes'
      }
    }
    stages {
        stage('Test') {
            steps {
                sh 'env'
            }
        }
    }
}