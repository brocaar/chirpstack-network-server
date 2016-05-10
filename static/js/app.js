var loraserver = angular.module('loraserver', [
    'ngRoute',

    'loraserverControllers'
    ]);

loraserver.config(["$provide", function($provide) {
    return $provide.decorator('$http', ['$delegate', function($delegate) {
        $delegate.rpc = function(method, parameters) {
            var data = {"method": method, "params": [parameters], "id" : 1};
            return $delegate.post('/rpc', data, {'headers':{'Content-Type': 'application/json'}});
        };
        return $delegate;
        }]);
    }]);

loraserver.config(['$routeProvider',
    function($routeProvider) {
        $routeProvider.
            when('/applications', {
                templateUrl: 'partials/applications.html',
                controller: 'ApplicationListCtrl'
            }).
            when('/applications/:application', {
                templateUrl: 'partials/applications.html',
                controller: 'ApplicationListCtrl'
            }).
            when('/nodes', {
                templateUrl: 'partials/nodes.html',
                controller: 'NodeListCtrl'
            }).
            when('/nodes/:node', {
                templateUrl: 'partials/nodes.html',
                controller: 'NodeListCtrl'
            }).
            when('/channels', {
                templateUrl: 'partials/channel_lists.html',
                controller: 'ChannelListListCtrl'
            }).
            when('/channels/:list', {
                templateUrl: 'partials/channel_list.html',
                controller: 'ChannelListCtrl'
            }).
            when('/api', {
                templateUrl: 'partials/api.html',
                controller: 'APICtrl'
            }).
            otherwise({
                redirectTo: '/applications'
            });
        }]);

var loraserverControllers = angular.module('loraserverControllers', []);

// display rpc docs
loraserverControllers.controller('APICtrl', ['$scope', '$http',
    function ($scope, $http) {
        $scope.page = 'api';
        $http.get('/rpc').success(function(data) {
            $scope.apis = data;
        });
    }]);

// manage applications
loraserverControllers.controller('ApplicationListCtrl', ['$scope', '$http', '$routeParams', '$route',
    function ($scope, $http, $routeParams, $route) {
        $scope.page = 'applications';
        $http.rpc('Application.GetList', {'limit': 9999, 'offset': 0}).success(function(data) {
                $scope.apps = data.result;
        });

        $scope.create = function(app) {
            if(app == null) {
                $('#createModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            } else {
                $http.rpc('Application.Create', app).success(function(data) {
                    if (data.error == null) {
                        $('#createModal').modal('hide');
                    }
                    $scope.error = data.error;
                });
            }
        };

        $scope.update = function(app) {
            $http.rpc('Application.Update', app).success(function(data) {
                if (data.error == null) {
                    $('#editModal').modal('hide');
                }
                $scope.error = data.error;
            });
        };

        $scope.delete = function(app) {
            if (confirm('Are you sure you want to delete ' + app.appEUI + '?')) {
                $http.rpc('Application.Delete', app.appEUI).success(function(data) {
                    if (data.error != null) {
                        alert(data.error);
                    }
                    $route.reload();
                });
            }
        };

        if($routeParams.application) {
            $http.rpc('Application.Get', $routeParams.application).success(function(data) {
                $scope.app = data.result;
                $('#editModal').modal().on('hidden.bs.modal', function() { history.go(-1); });
            });
        };
    }]);

// manage nodes
loraserverControllers.controller('NodeListCtrl', ['$scope', '$http', '$routeParams', '$route',
    function ($scope, $http, $routeParams, $route) {
        $http.rpc('Node.GetList', {'limit': 9999, 'offset': 0}).success(function(data) {
            $scope.nodes = data.result;
        });

        $http.rpc('ChannelList.GetList', {'limit': 9999, 'offset': 0}).success(function(data) {
            $scope.channelLists = data.result;
        });

        $scope.create = function(node) {
            if (node == null) {
                $('#createModal').modal().on('hidden.bs.modal', function(){
                    $route.reload();
                });
            } else {
                $http.rpc('Node.Create', node).success(function(data) {
                    if (data.error == null) {
                        $('#createModal').modal('hide');
                    }
                    $scope.error = data.error;
                });
            }
        };

        $scope.update = function(node) {
            $http.rpc('Node.Update', node).success(function(data) {
                if (data.error == null) {
                    $('#editModal').modal('hide');
                }
                $scope.error = data.error;
            });
        };

        $scope.delete = function(node) {
            if (confirm('Are you sure you want to delete ' + node.devEUI + '?')) {
                $http.rpc('Node.Delete', node.devEUI).success(function(data) {
                   if (data.error != null) {
                        alert(data.error);
                    }
                    $route.reload();
                });
            }
        };

        $scope.session = function(node) {
            $http.rpc('NodeSession.GetByDevEUI', node.devEUI).success(function(data) {
                $scope.ns = data.result;
                if ($scope.ns == null) {
                    $scope.ns = {
                        devEUI: node.devEUI,
                        appEUI: node.appEUI,
                        fCntUp: 0,
                        fCntDown: 0
                    };
                }
                $('#sessionModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            });
        };

        $scope.updateSession = function(ns) {
            $http.rpc('NodeSession.Update', ns).success(function(data) {
               if (data.error == null) {
                    $('#sessionModal').modal('hide');
                }
                $scope.error = data.error; 
            });
        };

        $scope.getRandomDevAddr = function(ns) {
            $http.rpc('NodeSession.GetRandomDevAddr', null).success(function(data) {
                ns.devAddr = data.result;
                $scope.error = data.error;
            });
        };

        if($routeParams.node) {
            $http.rpc('Node.Get', $routeParams.node).success(function(data) {
                $scope.node = data.result;
                $('#editModal').modal().on('hidden.bs.modal', function() { history.go(-1); });
            });
        };
        $scope.page = 'nodes';
    }]);

// manage channel lists
loraserverControllers.controller('ChannelListListCtrl', ['$scope', '$http', '$routeParams', '$route',
    function ($scope, $http, $routeParams, $route) {
        $scope.page = 'channels';
        $http.rpc('ChannelList.GetList', {'limit': 9999, 'offset': 0}).success(function(data) {
            $scope.channelLists = data.result;
        });

        $scope.createList = function(cl) {
            if (cl == null) {
                $('#createChannelListModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            } else {
                $http.rpc('ChannelList.Create', cl).success(function(data) {
                    if (data.error == null) {
                        $('#createChannelListModal').modal('hide');
                    }
                    $scope.error = data.error;
                });
            }
        };

        $scope.editList = function(cl) {
            $http.rpc('ChannelList.Get', cl.id).success(function(data) {
                $scope.channelList = data.result;
                $('#editChannelListModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            });
        };

        $scope.updateList = function(cl) {
            $http.rpc('ChannelList.Update', cl).success(function(data) {
                if (data.error == null) {
                    $('#editChannelListModal').modal('hide');
                }
                $scope.error = data.error;
            });
        };

        $scope.deleteList = function(cl) {
            if (confirm('Are you sure you want to delete ' + cl.name + '?')) {
                $http.rpc('ChannelList.Delete', cl.id).success(function(data) {
                    if (data.error != null) {
                        alert(data.error);
                    }
                    $route.reload();
                });
            }
        };
    }]);

// manage channel list
loraserverControllers.controller('ChannelListCtrl', ['$scope', '$http', '$routeParams', '$route',
    function ($scope, $http, $routeParams, $route) {
        $scope.page = 'channels';
        $http.rpc('ChannelList.Get', parseInt($routeParams.list)).success(function(data) {
            $scope.channelList = data.result;
        });
        $http.rpc('Channel.GetForChannelList', parseInt($routeParams.list)).success(function(data) {
            $scope.channels = data.result;
        });

        $scope.createChannel = function(c) {
            if (c == null) {
                $('#createChannelModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            } else {
                c.channelListID = $scope.channelList.id;
                $http.rpc('Channel.Create', c).success(function(data) {
                    if (data.error == null) {
                        $('#createChannelModal').modal('hide');
                    }
                    $scope.error = data.error;
                });
            }
        };

        $scope.editChannel = function(c) {
            $http.rpc('Channel.Get', c.id).success(function(data) {
                $scope.channel = data.result;
                $('#editChannelModal').modal().on('hidden.bs.modal', function() {
                    $route.reload();
                });
            });
        };

        $scope.updateChannel = function(c) {
            $http.rpc('Channel.Update', c).success(function(data) {
                if (data.error == null) {
                    $('#editChannelModal').modal('hide');
                }
                $scope.error = data.error;
            });
        };

        $scope.deleteChannel = function(c) {
            if (confirm('Are you sure you want to delete channel # ' + c.channel + '?')) {
                $http.rpc('Channel.Delete', c.id).success(function(data) {
                    if (data.error != null) {
                        alert(data.error);
                    }
                    $route.reload();
                });
            }
        };
    }]);
